//go:build integration

package release

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

func TestReleaseMetadataCapturesNamespacesAndModelTypes(t *testing.T) {
	for _, raw := range []*string{nil, strPtr(""), strPtr(" \t\n\u00a0\u2003"), strPtr(" \t\u3000https://example.org/concepts/\n")} {
		t.Run(fmt.Sprint(raw), func(t *testing.T) {
			pool := releaseTestPool(t)
			ctx, owner, project := seedZREL(t, pool)
			mustExec(t, pool, `UPDATE weave_projects SET concept_namespace=$1 WHERE id='ZREL'`, raw)
			mustExec(t, pool, `UPDATE weave_models SET model_type='auxiliary' WHERE project_id='ZREL'`)
			mustExec(t, pool, `INSERT INTO weave_models(id,project_id,model_type) VALUES ('ZREL-example','ZREL','example'),('ZREL-core','ZREL','core')`)
			service := NewService(NewPostgresStore(pool), pool, nil)
			if _, err := service.Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: "9.0.1"}); err != nil {
				t.Fatal(err)
			}
			want := "https://vocab.pletka.io/"
			if raw != nil && strings.TrimSpace(*raw) != "" {
				want = strings.TrimSpace(*raw)
			}
			var original readsession.ReleaseMetadata
			var closed *readsession.Session
			factory := readsession.NewFactory(pool)
			read := func(s *readsession.Session) error {
				var err error
				original, err = s.Metadata(ctx)
				closed = s
				return err
			}
			if err := factory.Read(ctx, auth.Release("9.0.1"), project, read); err != nil {
				t.Fatal(err)
			}
			got := original.ConceptNamespace()
			if (got == nil) != (raw == nil) || (raw != nil && *got != *raw) || original.EffectiveConceptNamespace() != want {
				t.Fatalf("namespace raw=%v effective=%q, want %v %q", got, original.EffectiveConceptNamespace(), raw, want)
			}
			if got != nil {
				*got = "mutated caller copy"
				if *original.ConceptNamespace() != *raw {
					t.Fatal("snapshot exposed a mutable pointer")
				}
			}
			for id, wantType := range map[string]string{"ZRELM.1": "auxiliary", "ZREL-example": "example", "ZREL-core": "core"} {
				if gotType, ok := original.ModelType(id); !ok || gotType != wantType {
					t.Fatalf("model %s = %q %v, want %q", id, gotType, ok, wantType)
				}
			}
			if _, ok := original.ModelType("foreign-model"); ok {
				t.Fatal("foreign model entered snapshot")
			}
			if _, err := closed.Metadata(ctx); err == nil {
				t.Fatal("closed session accepted")
			}
			if err := factory.Read(ctx, auth.Draft(), project, func(s *readsession.Session) error { _, err := s.Metadata(ctx); return err }); err == nil {
				t.Fatal("draft accepted as release metadata")
			}
			var zero readsession.Session
			if _, err := zero.Metadata(ctx); err == nil {
				t.Fatal("zero session accepted")
			}
			mustExec(t, pool, `UPDATE weave_projects SET concept_namespace='https://new.example/' WHERE id='ZREL'`)
			mustExec(t, pool, `UPDATE weave_models SET model_type='core' WHERE project_id='ZREL'`)
			if _, err := service.Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: "9.0.2"}); err != nil {
				t.Fatal(err)
			}
			mustExec(t, pool, `DELETE FROM weave_models WHERE project_id='ZREL'`)
			if err := factory.Read(ctx, auth.Release("9.0.1"), project, func(s *readsession.Session) error {
				after, err := s.Metadata(ctx)
				if err != nil {
					return err
				}
				if modelType, _ := after.ModelType("ZRELM.1"); modelType != "auxiliary" || after.EffectiveConceptNamespace() != want {
					t.Fatal("old release changed")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := factory.Read(ctx, auth.Release("9.0.2"), project, func(s *readsession.Session) error {
				after, err := s.Metadata(ctx)
				if err != nil {
					return err
				}
				if modelType, _ := after.ModelType("ZRELM.1"); modelType != "core" || after.EffectiveConceptNamespace() != "https://new.example/" {
					t.Fatal("new release reused old capture")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReleaseMetadataRejectsHistoricalEvidence(t *testing.T) {
	pool := releaseTestPool(t)
	ctx, owner, project := seedZREL(t, pool)
	mustExec(t, pool, `INSERT INTO weave_releases(project_id,version,created_by_id,archive_ownership_version) VALUES ('ZREL','9.0.1',$1,1)`, owner)
	// Even a populated field cannot retroactively certify an older release.
	mustExec(t, pool, `INSERT INTO weave_projects_archive(id,owner_id,version_number,concept_namespace,effective_concept_namespace) VALUES ('ZREL',$1,'9.0.1','https://invented/','https://invented/')`, owner)
	err := readsession.NewFactory(pool).Read(ctx, auth.Release("9.0.1"), project, func(s *readsession.Session) error { _, err := s.Metadata(ctx); return err })
	var missing *readsession.NotInReleaseError
	if !errors.As(err, &missing) {
		t.Fatalf("missing history = %v", err)
	}
	_, err = pool.Exec(ctx, `UPDATE weave_releases SET metadata_capture_version=1 WHERE project_id='ZREL'`)
	if err == nil {
		t.Fatal("historical release certified from current data")
	}
}

func TestReleaseMetadataStorageGuards(t *testing.T) {
	pool := releaseTestPool(t)
	ctx, owner, project := seedZREL(t, pool)
	if _, err := NewService(NewPostgresStore(pool), pool, nil).Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: "9.0.1"}); err != nil {
		t.Fatal(err)
	}
	// Legacy/unverified rows cannot be moved into a certified release either.
	mustExec(t, pool, `INSERT INTO weave_projects_archive(id,owner_id,version_number) VALUES ('ZREL','ZREL_OWNER','9.9.9')`)
	mustExec(t, pool, `INSERT INTO weave_models_archive(id,project_id,version_number) VALUES ('move-in','ZREL','9.9.9')`)
	for _, sql := range []string{
		`UPDATE weave_projects_archive SET effective_concept_namespace='https://changed/' WHERE id='ZREL'`,
		`UPDATE weave_projects_archive SET id='moved' WHERE id='ZREL'`,
		`DELETE FROM weave_projects_archive WHERE id='ZREL'`,
		`INSERT INTO weave_projects_archive(id,owner_id,version_number) VALUES ('ZREL','ZREL_OWNER','9.0.1')`,
		`UPDATE weave_models_archive SET model_type='example' WHERE project_id='ZREL'`,
		`UPDATE weave_models_archive SET project_id='moved' WHERE project_id='ZREL'`,
		`DELETE FROM weave_models_archive WHERE project_id='ZREL'`,
		`INSERT INTO weave_models_archive(id,project_id,version_number) VALUES ('extra','ZREL','9.0.1')`,
		`UPDATE weave_releases SET metadata_capture_version=0 WHERE project_id='ZREL'`,
		`UPDATE weave_releases SET archive_ownership_version=0 WHERE project_id='ZREL'`,
		`UPDATE weave_releases SET version='9.9.9' WHERE project_id='ZREL'`,
		`UPDATE weave_projects_archive SET version_number='9.0.1' WHERE id='ZREL' AND version_number='9.9.9'`,
		`UPDATE weave_models_archive SET version_number='9.0.1' WHERE id='move-in'`,
		`DELETE FROM weave_release_metadata_fences WHERE project_id='ZREL'`,
		`UPDATE weave_release_metadata_fences SET project_id='moved' WHERE project_id='ZREL'`,
		`TRUNCATE weave_release_metadata_fences`,
		`TRUNCATE weave_projects_archive`, `TRUNCATE weave_models_archive`,
	} {
		t.Run(sql, func(t *testing.T) {
			_, err := pool.Exec(ctx, sql)
			var pgerr *pgconn.PgError
			if !errors.As(err, &pgerr) || pgerr.Code != "23514" {
				t.Fatalf("mutation should fail closed: %v", err)
			}
		})
	}
	// Deleting a release still permits its archive rows to be cleaned up.
	mustExec(t, pool, `DELETE FROM weave_releases WHERE project_id='ZREL'`)
	mustExec(t, pool, `DELETE FROM weave_projects_archive WHERE id='ZREL'`)
	mustExec(t, pool, `DELETE FROM weave_models_archive WHERE project_id='ZREL'`)
}

func TestReleaseMetadataValidationRollsBackTheCut(t *testing.T) {
	for _, fault := range []string{"namespace", "missing-model", "wrong-model", "extra-model"} {
		t.Run(fault, func(t *testing.T) {
			pool := releaseTestPool(t)
			ctx, owner, project := seedZREL(t, pool)
			statements := map[string]string{
				"namespace":     `UPDATE weave_projects_archive SET effective_concept_namespace='https://wrong/' WHERE id='ZREL';`,
				"missing-model": `DELETE FROM weave_models_archive WHERE project_id='ZREL';`,
				"wrong-model":   `UPDATE weave_models_archive SET model_type='example' WHERE project_id='ZREL';`,
				"extra-model":   `INSERT INTO weave_models_archive(id,project_id,version_number) VALUES ('extra','ZREL',NEW.release_version);`,
			}
			mustExec(t, pool, `CREATE FUNCTION corrupt_metadata_cut() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN `+statements[fault]+` RETURN NEW; END $$`)
			mustExec(t, pool, `CREATE TRIGGER corrupt_metadata_cut AFTER INSERT ON weave_change_set FOR EACH ROW WHEN (NEW.project_id='ZREL' AND NEW.kind='release') EXECUTE FUNCTION corrupt_metadata_cut()`)
			t.Cleanup(func() {
				mustExec(t, pool, `DROP TRIGGER corrupt_metadata_cut ON weave_change_set`)
				mustExec(t, pool, `DROP FUNCTION corrupt_metadata_cut()`)
			})
			_, err := NewService(NewPostgresStore(pool), pool, nil).Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: "9.0.1"})
			var pgerr *pgconn.PgError
			if !errors.As(err, &pgerr) || pgerr.Code != "23514" {
				t.Fatalf("capture error = %v", err)
			}
			var remaining int
			mustScan(t, pool, `SELECT (SELECT count(*) FROM weave_releases WHERE project_id='ZREL')+(SELECT count(*) FROM weave_projects_archive WHERE id='ZREL')+(SELECT count(*) FROM weave_models_archive WHERE project_id='ZREL')+(SELECT count(*) FROM weave_change_set WHERE project_id='ZREL')`, nil, &remaining)
			if remaining != 0 {
				t.Fatalf("failed cut left %d rows", remaining)
			}
		})
	}
}

func TestReleaseMetadataAcceptsEmptyModelMembership(t *testing.T) {
	pool := releaseTestPool(t)
	// Empty model membership is valid evidence, not an absent section.
	ctx, owner, project := seedZREL(t, pool)
	mustExec(t, pool, `DELETE FROM weave_field_overrides WHERE project_id='ZREL'`)
	mustExec(t, pool, `DELETE FROM weave_models WHERE project_id='ZREL'`)
	if _, err := NewService(NewPostgresStore(pool), pool, nil).Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: "9.0.1"}); err != nil {
		t.Fatal(err)
	}
	if err := readsession.NewFactory(pool).Read(ctx, auth.Release("9.0.1"), project, func(s *readsession.Session) error { _, err := s.Metadata(ctx); return err }); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataCaptureRequiresSnapshotIsolation(t *testing.T) {
	pool := releaseTestPool(t)
	ctx, owner, project := seedZREL(t, pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('ZREL','9.0.1',$1)`, owner); err != nil {
		t.Fatal(err)
	}
	if err := snapshotOwnedState(ctx, tx, project, "9.0.1"); err != nil {
		t.Fatal(err)
	}
	err = sqlcgen.New(tx).WeaveFinalizeReleaseCapture(ctx, sqlcgen.WeaveFinalizeReleaseCaptureParams{ProjectID: project, Version: "9.0.1"})
	if err == nil || !strings.Contains(err.Error(), "repeatable-read") {
		t.Fatalf("read-committed capture = %v", err)
	}
}
