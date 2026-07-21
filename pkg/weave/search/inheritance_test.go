//go:build integration

package search

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	return testdb.Pool(t)
}

func seedActor(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	id := ids.GenerateULID()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO weave_actors (id, type, display_name, system_name, slug, email)
		VALUES ($1, 'person', $2, $2, $2, $3)
	`, id, name, name+"@test.local"); err != nil {
		t.Fatalf("seed actor %s: %v", name, err)
	}
	return id
}

func TestWeaveSearchFields_UsesProjectInheritanceScope(t *testing.T) {
	pool := testPool(t)
	queries := sqlcgen.New(pool)
	ctx := context.Background()

	ownerID := seedActor(t, pool, "TEST_SEARCH_INHERIT_OWNER")
	parentID := "TEST_SEARCH_PARENT"
	childID := "TEST_SEARCH_CHILD"
	fieldID := "TEST_SEARCH_FIELD"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_field_overrides WHERE project_id LIKE 'TEST_SEARCH_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields WHERE id = $1`, fieldID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_SEARCH_%' OR parent_project_id LIKE 'TEST_SEARCH_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_SEARCH_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_projects (id, owner_id)
		VALUES ($1, $3), ($2, $3)
	`, parentID, childID, ownerID); err != nil {
		t.Fatalf("seed projects: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, canonical_order)
		VALUES ($1, $2, true, 0)
	`, childID, parentID); err != nil {
		t.Fatalf("seed inheritance: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_fields (
			id, semantic_id, system_name, ui_name, description, status, project_id
		) VALUES (
			$1, 'TEST.SEARCH.1', 'actor-name', '{"en":"Actor Name"}'::jsonb,
			'{"en":"Search inheritance field"}'::jsonb, 'draft', $2
		)
	`, fieldID, parentID); err != nil {
		t.Fatalf("seed field: %v", err)
	}

	rows, err := queries.WeaveSearchFields(ctx, sqlcgen.WeaveSearchFieldsParams{
		ProjectID:         childID,
		Scope:             "inherited",
		Search:            "Actor",
		PathLocalName:     "",
		PathPrefix:        "",
		Category:          "",
		ExpectedValueType: "",
		OntologyClass:     "",
		OntologyPrefix:    "",
		PathFieldIds:      []string{},
		SortBy:            "relevance",
		ResultLimit:       25,
		ResultOffset:      0,
	})
	if err != nil {
		t.Fatalf("WeaveSearchFields: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].ID != fieldID {
		t.Fatalf("row id=%q want %q", rows[0].ID, fieldID)
	}
	if rows[0].ProjectID != parentID {
		t.Fatalf("row project_id=%q want inherited parent %q", rows[0].ProjectID, parentID)
	}

	count, err := queries.WeaveCountSearchFields(ctx, sqlcgen.WeaveCountSearchFieldsParams{
		ProjectID:         childID,
		Scope:             "inherited",
		Search:            "Actor",
		PathLocalName:     "",
		PathPrefix:        "",
		Category:          "",
		ExpectedValueType: "",
		OntologyClass:     "",
		OntologyPrefix:    "",
		PathFieldIds:      []string{},
	})
	if err != nil {
		t.Fatalf("WeaveCountSearchFields: %v", err)
	}
	if count != 1 {
		t.Fatalf("count=%d want 1", count)
	}
}

func TestInheritedSearch_UsesPinnedParentReleaseState(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	h := NewHandler(store, slog.Default())
	r := chi.NewRouter()
	h.Mount(r)
	ctx := context.Background()

	ownerID := seedActor(t, pool, "TEST_SEARCH_PINNED_OWNER")
	parentID := "TEST_SEARCH_PINNED_PARENT"
	childID := "TEST_SEARCH_PINNED_CHILD"
	fieldID := "TEST_SEARCH_PINNED_FIELD"
	version := "0.9.0-test"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields_archive WHERE project_id LIKE 'TEST_SEARCH_PINNED_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields WHERE id = $1`, fieldID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_SEARCH_PINNED_%' OR parent_project_id LIKE 'TEST_SEARCH_PINNED_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_SEARCH_PINNED_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_projects (id, owner_id)
		VALUES ($1, $3), ($2, $3)
	`, parentID, childID, ownerID); err != nil {
		t.Fatalf("seed projects: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, canonical_order, source_mode, source_version)
		VALUES ($1, $2, true, 0, 'release', $3)
	`, childID, parentID, version); err != nil {
		t.Fatalf("seed pinned inheritance: %v", err)
	}

	hotPath, _ := json.Marshal([]domain.PathElement{
		{Type: "property", Prefix: "crm", LocalName: "P1_is_identified_by", Position: 0},
		{Type: "class", Prefix: "crm", LocalName: "E42_Identifier", Position: 1},
	})
	archivedPath, _ := json.Marshal([]domain.PathElement{
		{Type: "property", Prefix: "crm", LocalName: "P2_has_type", Position: 0},
		{Type: "class", Prefix: "crm", LocalName: "E55_Type", Position: 1},
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_fields (
			id, semantic_id, system_name, ui_name, description, status, project_id, path_elements
		) VALUES (
			$1, 'TEST.SEARCH.PINNED.1', 'modern-name', '{"en":"Modern Name"}'::jsonb,
			'{"en":"Hot draft field"}'::jsonb, 'draft', $2, $3::jsonb
		)
	`, fieldID, parentID, string(hotPath)); err != nil {
		t.Fatalf("seed hot field: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_fields_archive (
			id, semantic_id, system_name, ui_name, description, status, project_id, version_number, path_elements
		) VALUES (
			$1, 'TEST.SEARCH.PINNED.1', 'legacy-name', '{"en":"Legacy Name"}'::jsonb,
			'{"en":"Pinned release field"}'::jsonb, 'draft', $2, $3, $4::jsonb
		)
	`, fieldID, parentID, version, string(archivedPath)); err != nil {
		t.Fatalf("seed archived field: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+childID+"/search?type=field&scope=inherited&q=Legacy", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy search status=%d body=%s", rec.Code, rec.Body.String())
	}
	var legacyResp domain.SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &legacyResp); err != nil {
		t.Fatalf("decode legacy search: %v", err)
	}
	if len(legacyResp.Items) != 1 || legacyResp.Items[0].ID != fieldID {
		t.Fatalf("legacy search items=%+v", legacyResp.Items)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+childID+"/search?type=field&scope=inherited&q=Modern", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("modern search status=%d body=%s", rec.Code, rec.Body.String())
	}
	var modernResp domain.SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &modernResp); err != nil {
		t.Fatalf("decode modern search: %v", err)
	}
	if len(modernResp.Items) != 0 {
		t.Fatalf("expected no modern results from pinned parent, got %+v", modernResp.Items)
	}
}

func TestPathSuggestions_UsePinnedParentReleaseState(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	h := NewHandler(store, slog.Default())
	r := chi.NewRouter()
	h.Mount(r)
	ctx := context.Background()

	ownerID := seedActor(t, pool, "TEST_PATH_PINNED_OWNER")
	parentID := "TEST_PATH_PINNED_PARENT"
	childID := "TEST_PATH_PINNED_CHILD"
	fieldID := "TEST_PATH_PINNED_FIELD"
	version := "0.9.0-test"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields_archive WHERE project_id LIKE 'TEST_PATH_PINNED_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields WHERE id = $1`, fieldID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_PATH_PINNED_%' OR parent_project_id LIKE 'TEST_PATH_PINNED_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_PATH_PINNED_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_projects (id, owner_id)
		VALUES ($1, $3), ($2, $3)
	`, parentID, childID, ownerID); err != nil {
		t.Fatalf("seed projects: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, canonical_order, source_mode, source_version)
		VALUES ($1, $2, true, 0, 'release', $3)
	`, childID, parentID, version); err != nil {
		t.Fatalf("seed pinned inheritance: %v", err)
	}

	hotPath, _ := json.Marshal([]domain.PathElement{
		{Type: "property", Prefix: "crm", LocalName: "P1_is_identified_by", Position: 0},
		{Type: "class", Prefix: "crm", LocalName: "E42_Identifier", Position: 1},
	})
	archivedPath, _ := json.Marshal([]domain.PathElement{
		{Type: "property", Prefix: "crm", LocalName: "P2_has_type", Position: 0},
		{Type: "class", Prefix: "crm", LocalName: "E55_Type", Position: 1},
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_fields (
			id, semantic_id, system_name, ui_name, description, status, project_id, path_elements
		) VALUES (
			$1, 'TEST.PATH.PINNED.1', 'modern-path', '{"en":"Modern Path"}'::jsonb,
			'{"en":"Hot path field"}'::jsonb, 'draft', $2, $3::jsonb
		)
	`, fieldID, parentID, string(hotPath)); err != nil {
		t.Fatalf("seed hot field: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_fields_archive (
			id, semantic_id, system_name, ui_name, description, status, project_id, version_number, path_elements
		) VALUES (
			$1, 'TEST.PATH.PINNED.1', 'legacy-path', '{"en":"Legacy Path"}'::jsonb,
			'{"en":"Pinned path field"}'::jsonb, 'draft', $2, $3, $4::jsonb
		)
	`, fieldID, parentID, version, string(archivedPath)); err != nil {
		t.Fatalf("seed archived field: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+childID+"/path-suggestions?scope=inherited&current_path=crm:P2_has_type", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("path suggestions status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp domain.PathSuggestionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode path suggestions: %v", err)
	}
	if len(resp.Suggestions) != 1 || resp.Suggestions[0].Display != "crm:E55_Type" {
		t.Fatalf("unexpected suggestions: %+v", resp.Suggestions)
	}
}
