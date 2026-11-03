//go:build integration

package project_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

// The project slice mounts its own routes, so it must resolve the content
// version the same way every other project-scoped slice does: a non-editor
// reading a public project gets its latest release, not the working draft.
func TestProjectRoutesServeReleaseToAnonymousReaders(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	const projectID = "RVSADO"
	seedReleasedProjectWithLiveAdoption(t, pool, projectID)
	router := newSaveConflictRouter(pool)
	path := "/projects/" + projectID + "/adoptions"

	for _, tc := range []struct {
		name string
		ctx  context.Context
		path string
		want int
	}{
		{"anonymous reads the release", ctx, path, 0},
		{"editor reads the draft", editorAuthContext(), path, 1},
		{"explicit version pins the release for an editor", editorAuthContext(), path + "?version=1.0.0", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil).WithContext(tc.ctx)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Adoptions []domain.Adoption `json:"adoptions"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Adoptions) != tc.want {
				t.Fatalf("got %d adoptions, want %d", len(body.Adoptions), tc.want)
			}
		})
	}
}

// seedReleasedProjectWithLiveAdoption creates a public project with one
// field, cuts release 1.0.0, then records an adoption only the draft has.
func seedReleasedProjectWithLiveAdoption(t *testing.T, pool *pgxpool.Pool, projectID string) {
	t.Helper()
	ctx := context.Background()
	t.Cleanup(func() {
		bg := context.Background()
		for _, sql := range []string{
			`DELETE FROM weave_adoptions WHERE project_id=$1`,
			`DELETE FROM weave_adoptions_archive WHERE project_id=$1`,
			`DELETE FROM weave_change_set WHERE project_id=$1`,
			`DELETE FROM weave_releases WHERE project_id=$1`,
			`DELETE FROM weave_fields_archive WHERE project_id=$1`,
			`DELETE FROM weave_fields WHERE project_id=$1`,
			`DELETE FROM weave_projects WHERE id=$1`,
		} {
			_, _ = pool.Exec(bg, sql, projectID)
		}
	})
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id, visibility) VALUES ($1, 'unite', 'public')`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := field.NewPostgresStore(pool).Create(ctx, &domain.Field{
		Entity:       domain.Entity{ID: ids.GenerateULID(), ProjectID: projectID, SemanticID: projectID + "F.1", SystemName: "released", UIName: domain.Translations{"en": "Released"}, Status: domain.StatusPublished},
		PathElements: []domain.PathElement{},
	}); err != nil {
		t.Fatalf("seed field: %v", err)
	}
	releaseCtx := weaveauth.WithPrincipal(weaveauth.WithSnapshot(ctx, &weaveauth.AuthSnapshot{IsSuperAdmin: true}), &weaveauth.Principal{ActorID: "unite"})
	if _, err := release.NewService(nil, pool, nil).Create(releaseCtx, projectID, release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatalf("create release: %v", err)
	}
	if err := weave.NewPostgresStore(pool).Adoptions().ReplaceForContext(ctx, projectID, "project", projectID, []domain.Adoption{{
		ProjectID: projectID, ContextEntityType: "project", ContextEntityID: projectID, EntityType: "field", SourceProjectID: "LA", SourceEntityID: "LAF.230",
	}}); err != nil {
		t.Fatalf("seed live adoption: %v", err)
	}
}
