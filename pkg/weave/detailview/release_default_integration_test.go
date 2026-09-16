//go:build integration

package detailview_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/publication"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

// TestReleaseDefault_FieldRoute proves Task 4's wiring end-to-end: mounting
// auth.ResolveContentVersion (backed by publication.NewReader) after
// auth.WithProjectResource on a project-scoped slice route — exactly the
// chain router.mountSlice installs via router.ProjectMiddlewareHost — makes
// a public project's non-editor reader default to the latest release
// instead of the hot draft, while an editor (here: an org-inherited owner)
// keeps seeing hot, and an explicit ?version= is unaffected either way.
//
// The route under test is the field slice's own GET /api/{fieldID} (mounted
// at /projects/{projectID}/fields), not detailview's entity-view API: buildField
// in this package's handler.go always reads the live (hot) field row — it
// never consults auth.ProjectVersionFromContext for content, only for URLs
// and the publication badge — so it can't demonstrate a hot/release content
// difference. field.Service.Get, by contrast, already branches on
// auth.ProjectVersionFromContext (pre-existing support for explicit
// ?version=), making it the real proof point for the release-default
// middleware this task wires up.
func TestReleaseDefault_FieldRoute(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	const projectID = "RELDEF"
	fieldID := ids.GenerateULID()

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_change_set WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_releases WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields_archive WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id=$1`, projectID)
	})

	// "unite" is a fixture organization actor seeded for every package's
	// testdb clone (internal/testdb/fixture_identities.go). Owning the
	// project with it lets us grant org-inherited edit access below via
	// Roles["org:unite"].
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_projects (id, owner_id, visibility) VALUES ($1, 'unite', 'public')
	`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	fieldStore := field.NewPostgresStore(pool)
	weaveStore := weave.NewPostgresStore(pool)

	seedField := &domain.Field{
		Entity: domain.Entity{
			ID:         fieldID,
			ProjectID:  projectID,
			SemanticID: "RELDEFF.1",
			SystemName: "original_name",
			UIName:     domain.Translations{"en": "Original Name"},
			Status:     domain.StatusPublished,
		},
		// path_elements is jsonb NOT NULL, walked by
		// jsonb_array_elements() elsewhere — a nil slice marshals to JSON
		// null, not "[]", and trips "cannot extract elements from a
		// scalar". An explicit empty slice avoids that.
		PathElements: []domain.PathElement{},
	}
	if err := fieldStore.Create(ctx, seedField); err != nil {
		t.Fatalf("seed field: %v", err)
	}

	// Create a release — snapshots the live field (still "Original Name")
	// into weave_fields_archive under version 1.0.0.
	releaseCtx := auth.WithPrincipal(
		auth.WithSnapshot(ctx, &auth.AuthSnapshot{IsSuperAdmin: true}),
		&auth.Principal{ActorID: "unite"},
	)
	releaseSvc := release.NewService(nil, pool, nil)
	if _, err := releaseSvc.Create(releaseCtx, projectID, release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatalf("create release: %v", err)
	}

	// Mutate the live field so hot diverges from the release snapshot.
	seedField.UIName = domain.Translations{"en": "Renamed Hot"}
	seedField.SystemName = "renamed_hot"
	if err := fieldStore.Update(ctx, seedField); err != nil {
		t.Fatalf("rename live field: %v", err)
	}

	// Mount the field slice behind the same middleware chain
	// router.mountSlice installs when a ProjectMiddlewareHost carries a
	// LatestRelease reader (router.go: WithProjectVersionContext ->
	// WithProjectResource -> ResolveContentVersion).
	pub := publication.NewReader(pool)
	fieldHost := field.Host{
		Service:  field.NewService(fieldStore, nil, nil, nil, weaveStore.Projects(), nil, nil, nil, nil, nil),
		Projects: weaveStore.Projects(),
	}
	sub := chi.NewMux()
	sub.Use(auth.WithProjectVersionContext)
	sub.Use(auth.WithProjectResource(weaveStore))
	sub.Use(auth.ResolveContentVersion(pub))
	field.Mount(sub, fieldHost)

	router := chi.NewMux()
	router.Mount("/projects/{projectID}/fields", sub)

	fieldPath := "/projects/" + projectID + "/fields/api/" + fieldID

	t.Run("anonymous default sees the released name", func(t *testing.T) {
		name := fetchFieldName(t, router, fieldPath, nil)
		if name != "Original Name" {
			t.Fatalf("ui_name.en = %q, want the released name %q", name, "Original Name")
		}
	})

	t.Run("org-inherited owner default sees the hot name", func(t *testing.T) {
		snap := &auth.AuthSnapshot{
			ActorID: "org-inherited-owner",
			Roles:   map[string]string{"org:unite": "owner"},
		}
		name := fetchFieldName(t, router, fieldPath, snap)
		if name != "Renamed Hot" {
			t.Fatalf("ui_name.en = %q, want the hot name %q", name, "Renamed Hot")
		}
	})

	t.Run("explicit version query is unaffected", func(t *testing.T) {
		name := fetchFieldName(t, router, fieldPath+"?version=1.0.0", nil)
		if name != "Original Name" {
			t.Fatalf("ui_name.en = %q, want the released name %q", name, "Original Name")
		}
	})
}

func fetchFieldName(t *testing.T, router chi.Router, path string, snap *auth.AuthSnapshot) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if snap != nil {
		req = req.WithContext(auth.WithSnapshot(req.Context(), snap))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, body = %s", path, rec.Code, rec.Body.String())
	}
	var body struct {
		UIName map[string]string `json:"ui_name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal field response: %v\n%s", err, rec.Body.String())
	}
	return body.UIName["en"]
}
