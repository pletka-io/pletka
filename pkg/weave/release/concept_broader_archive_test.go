//go:build integration

package release

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// TestCreate_ArchivesConceptBroaderEdges proves a release snapshots the
// scheme-scoped hierarchy edges into weave_concept_broader_archive at the new
// version, mirroring the concept-list/entry archival (#3599).
func TestCreate_ArchivesConceptBroaderEdges(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	queries := sqlcgen.New(pool)

	const (
		ownerID   = "TEST_CBA_OWNER"
		projectID = "TEST_CBA_PROJ"
	)

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_concept_broader_archive WHERE scheme_id = 'CBA_LIST'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_concept_broader WHERE scheme_id = 'CBA_LIST'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_concept_list_entries WHERE concept_list_id = 'CBA_LIST'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_concept_lists WHERE id = 'CBA_LIST'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_vocabulary_entries WHERE vocabulary_id = 'CBA_VOC'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_vocabularies WHERE id = 'CBA_VOC'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_releases WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_categories WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID: ownerID, Type: "organization", DisplayName: "CBA Org", Slug: "cba-org", Visibility: "private",
	}); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID: projectID, UiName: []byte(`{"en":"CBA"}`), Status: "draft", OwnerID: ownerID, Visibility: "private",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := queries.WeaveCreateCategory(ctx, sqlcgen.WeaveCreateCategoryParams{
		ID: "CBA_CAT", SemanticID: strPtr("TEST_CBA.CAT.1"), SystemName: strPtr("cba-cat"),
		UiName: []byte(`{"en":"Cat"}`), Description: []byte(`{"en":"cat"}`), Status: "draft", ProjectID: projectID,
	}); err != nil {
		t.Fatalf("create category: %v", err)
	}

	seedHierarchy(t, ctx, pool, projectID)

	authCtx := weaveauth.WithSnapshot(ctx, &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	authCtx = weaveauth.WithPrincipal(authCtx, &weaveauth.Principal{ActorID: ownerID})

	svc := NewService(nil, pool, nil)
	rel, err := svc.Create(authCtx, projectID, CreateInput{Version: "1.0.0"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var archivedVersion string
	if err := pool.QueryRow(ctx, `
SELECT version_number FROM weave_concept_broader_archive
WHERE id = 'CBA_EDGE' AND scheme_id = 'CBA_LIST'`).Scan(&archivedVersion); err != nil {
		t.Fatalf("broader edge should be archived: %v", err)
	}
	if archivedVersion != rel.Version {
		t.Fatalf("edge archived at %q, want release version %q", archivedVersion, rel.Version)
	}
}

func seedHierarchy(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID string) {
	t.Helper()
	stmts := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO weave_vocabularies (id, connector_type, project_id) VALUES ('CBA_VOC','local',$1)`, []any{projectID}},
		{`INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label) VALUES ('CBA_A','CBA_VOC','pletka:concept/A','{"en":"Crimson"}')`, nil},
		{`INSERT INTO weave_vocabulary_entries (id, vocabulary_id, uri, label) VALUES ('CBA_B','CBA_VOC','pletka:concept/B','{"en":"Red"}')`, nil},
		{`INSERT INTO weave_concept_lists (id, project_id) VALUES ('CBA_LIST',$1)`, []any{projectID}},
		{`INSERT INTO weave_concept_broader (id, concept_id, broader_id, scheme_id) VALUES ('CBA_EDGE','CBA_A','CBA_B','CBA_LIST')`, nil},
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatalf("seed hierarchy (%s): %v", s.sql, err)
		}
	}
}
