//go:build integration

package readsession_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	archive "github.com/pletka-io/pletka/pkg/database/readsql/dependencyarchive"
	"github.com/pletka-io/pletka/pkg/database/releasedeps"
)

func TestBatchMembershipMatchesAllArchiveKindsAndExactSource(t *testing.T) {
	ctx := t.Context()
	tx, err := testdb.Pool(t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Uncertified history lets the query matrix exercise damaged membership
	// without bypassing guards on a completed release. ReadCaptured's marker,
	// authorization and pin checks are tested separately through the factory.
	exec(`INSERT INTO weave_projects(id,owner_id) VALUES ('BATCH_ROOT','unite'),('BATCH_SOURCE','unite'),('BATCH_OTHER','unite')`)
	exec(`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('BATCH_ROOT','1','unite'),('BATCH_ROOT','2','unite'),('BATCH_SOURCE','1','unite'),('BATCH_OTHER','1','unite')`)
	exec(`INSERT INTO weave_release_project_dependencies(project_id,version_number,source_project_id,source_version) VALUES ('BATCH_ROOT','1','BATCH_SOURCE','1'),('BATCH_ROOT','1','BATCH_OTHER','1'),('BATCH_ROOT','2','BATCH_SOURCE','1'),('BATCH_OTHER','1','BATCH_SOURCE','1')`)
	// Invalid members in another consumer/version must not poison this read.
	exec(`INSERT INTO weave_release_dependency_entities(project_id,version_number,entity_type,entity_id,source_project_id,source_version) VALUES ('BATCH_ROOT','2','unknown','outside-version','BATCH_SOURCE','1'),('BATCH_OTHER','1','unknown','outside-project','BATCH_SOURCE','1')`)
	for _, sql := range []string{
		`INSERT INTO weave_projects_archive(id,owner_id,version_number) VALUES ('BATCH_SOURCE','unite','1')`,
		`INSERT INTO weave_fields_archive(id,project_id,version_number) VALUES ('BATCH.item','BATCH_SOURCE','1'),('BATCH.version-only','BATCH_SOURCE','2')`,
		`INSERT INTO weave_models_archive(id,project_id,version_number) VALUES ('BATCH.item','BATCH_SOURCE','1')`,
		`INSERT INTO weave_collections_archive(id,project_id,version_number) VALUES ('BATCH.item','BATCH_SOURCE','1')`,
		`INSERT INTO weave_categories_archive(id,project_id,version_number) VALUES ('BATCH.item','BATCH_SOURCE','1')`,
		`INSERT INTO weave_concept_lists_archive(id,project_id,version_number) VALUES ('BATCH.item','BATCH_SOURCE','1')`,
		`INSERT INTO weave_examples_archive(id,project_id,entity_type,entity_id,version_number) VALUES ('BATCH.item','BATCH_SOURCE','collection','BATCH.item','1')`,
		`INSERT INTO weave_field_overrides_archive(id,field_id,project_id,version_number) VALUES (900000001,'BATCH.item','BATCH_SOURCE','1')`,
		`INSERT INTO weave_vocabularies_archive(id,project_id,connector_type,version_number) VALUES ('BATCH.item','BATCH_SOURCE','local','1'),('BATCH.v2','BATCH_SOURCE','local','2'),('BATCH.foreign','BATCH_OTHER','local','1')`,
		`INSERT INTO weave_vocabulary_entries_archive(id,vocabulary_id,uri,label,version_number) VALUES ('BATCH.item','BATCH.item','urn:one','{}','1'),('BATCH.wrong-parent-version','BATCH.v2','urn:two','{}','1'),('BATCH.foreign-entry','BATCH.foreign','urn:three','{}','1')`,
		`INSERT INTO weave_fields(id,project_id) VALUES ('BATCH.live-only','BATCH_SOURCE')`,
	} {
		exec(sql)
	}
	q := archive.New(tx)
	params := archive.WeaveFirstMissingReleaseDependencyEntityParams{ProjectID: "BATCH_ROOT", Version: "1"}
	kinds := []string{"project", "field", "model", "collection", "category", "concept_list", "example", "override", "vocabulary", "vocabulary_entry"}
	for _, kind := range kinds {
		id := "BATCH.item"
		if kind == "project" {
			id = "BATCH_SOURCE"
		}
		if kind == "override" {
			id = "900000001"
		}
		exec(`INSERT INTO weave_release_dependency_entities(project_id,version_number,entity_type,entity_id,source_project_id,source_version) VALUES ('BATCH_ROOT','1',$1,$2,'BATCH_SOURCE','1')`, kind, id)
	}
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		exec(`SELECT set_config('plan_cache_mode',$1,true)`, mode)
		if _, err := q.WeaveFirstMissingReleaseDependencyEntity(ctx, params); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("valid complete membership rejected: %v", err)
		}
		type identityCase struct{ kind, id, source string }
		cases := make([]identityCase, 0, len(kinds)+6)
		for _, kind := range kinds {
			cases = append(cases, identityCase{kind, "BATCH.absent", "BATCH_SOURCE"})
		}
		cases = append(cases,
			identityCase{"unknown", "BATCH.item", "BATCH_SOURCE"},
			identityCase{"field", "BATCH.item", "BATCH_OTHER"},
			identityCase{"field", "BATCH.version-only", "BATCH_SOURCE"},
			identityCase{"field", "BATCH.live-only", "BATCH_SOURCE"},
			identityCase{"vocabulary_entry", "BATCH.wrong-parent-version", "BATCH_SOURCE"},
			identityCase{"vocabulary_entry", "BATCH.foreign-entry", "BATCH_SOURCE"},
		)
		for _, tc := range cases {
			// Temporarily replace a valid key too, to test the same ID owned by a
			// different source project with the same textual version.
			exec(`SAVEPOINT invalid_member`)
			exec(`DELETE FROM weave_release_dependency_entities WHERE project_id='BATCH_ROOT' AND version_number='1' AND entity_type=$1 AND entity_id=$2`, tc.kind, tc.id)
			exec(`INSERT INTO weave_release_dependency_entities(project_id,version_number,entity_type,entity_id,source_project_id,source_version) VALUES ('BATCH_ROOT','1',$1,$2,$3,'1')`, tc.kind, tc.id, tc.source)
			found, err := q.WeaveReleaseContainsDependencyEntity(ctx, archive.WeaveReleaseContainsDependencyEntityParams{ProjectID: tc.source, Version: "1", EntityType: tc.kind, EntityID: tc.id})
			if err != nil || found {
				t.Fatalf("invalid scalar identity %v: found=%v err=%v", tc, found, err)
			}
			missing, err := q.WeaveFirstMissingReleaseDependencyEntity(ctx, params)
			if err != nil || missing.EntityType != tc.kind || missing.EntityID != tc.id || missing.SourceProjectID != tc.source || missing.SourceVersion != "1" {
				t.Fatalf("batch lost invalid identity %v: %+v %v", tc, missing, err)
			}
			exec(`ROLLBACK TO SAVEPOINT invalid_member`)
			exec(`RELEASE SAVEPOINT invalid_member`)
		}
	}
}

func TestCapturedBulkMembershipUsesOneBatchAndSurvivesDraftDeletion(t *testing.T) {
	source, consumer := newFixture(t, "BULK_SOURCE"), newFixture(t, "BULK_CONSUMER")
	const count = 512
	if _, err := source.pool.Exec(source.ctx, `INSERT INTO weave_fields(id,project_id) SELECT $1||'.bulk.'||n,$1 FROM generate_series(1,$2::integer) n`, source.project, count); err != nil {
		t.Fatal(err)
	}
	source.cut(t)
	if _, err := consumer.pool.Exec(consumer.ctx, `INSERT INTO weave_field_overrides(field_id,project_id) SELECT $1||'.bulk.'||n,$2 FROM generate_series(1,$3::integer) n`, source.project, consumer.project, count); err != nil {
		t.Fatal(err)
	}
	consumer.cut(t)
	trace := &ontologyTrace{pids: map[uint32]bool{}}
	config := consumer.pool.Config()
	config.ConnConfig.Tracer = trace
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(consumer.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	read := func(policy readsession.SourceAuthorization) ([]releasedeps.Member, error) {
		trace.queries = nil
		var members []releasedeps.Member
		err := readsession.NewFactory(pool).WithSourceAuthorization(policy).Read(consumer.ctx, auth.Release("1.0.0"), consumer.project, func(s *readsession.Session) error {
			var err error
			members, err = s.Membership(consumer.ctx)
			return err
		})
		return members, err
	}
	before, err := read(auth.CheckProjectRead)
	if err != nil || len(before) != count {
		t.Fatalf("bulk membership: %d, %v", len(before), err)
	}
	assertMembershipQueries(t, trace.queries, 1, 1)
	if _, err := source.pool.Exec(source.ctx, `DELETE FROM weave_fields WHERE project_id=$1 AND id LIKE $1||'.bulk.%'`, source.project); err != nil {
		t.Fatal(err)
	}
	if err := cutVersion(t, source, "2.0.0"); err != nil {
		t.Fatal(err)
	}
	after, err := read(auth.CheckProjectRead)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(before, after); diff != "" {
		t.Fatal(diff)
	}
	assertMembershipQueries(t, trace.queries, 1, 1)
	denied := errors.New("source access revoked")
	partial, err := read(func(context.Context, auth.Resource) error { return denied })
	if !errors.Is(err, denied) || len(partial) != 0 {
		t.Fatalf("unauthorized membership escaped: %v", err)
	}
	assertMembershipQueries(t, trace.queries, 0, 0)
}

func assertMembershipQueries(t *testing.T, queries []string, wantScalar, wantBatch int) {
	t.Helper()
	scalar, batch := 0, 0
	for _, query := range queries {
		if strings.HasPrefix(query, "-- name: WeaveReleaseContainsDependencyEntity ") {
			scalar++
		}
		if strings.HasPrefix(query, "-- name: WeaveFirstMissingReleaseDependencyEntity ") {
			batch++
		}
	}
	if scalar != wantScalar || batch != wantBatch {
		t.Fatalf("individual queries=%d want=%d, batch queries=%d want=%d", scalar, wantScalar, batch, wantBatch)
	}
}
