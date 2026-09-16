//go:build integration

package search

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/publication"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

// TestSearch_ReleaseDefault_ServesReleasedSnapshot proves Task 6's fix: search
// no longer refuses release-mode requests with searchUnavailableInReleaseMode
// (removed by this change). Instead, mounting auth.ResolveContentVersion
// (backed by publication.NewReader) after auth.RequireProjectRead — the same
// chain entityschema.Handler.Mount installs — makes a public project's
// non-editor viewer default onto the latest release, and the handler routes
// that resolved version through the pre-existing archived-search machinery
// (searchArchivedFields / resolveProjectTargets) to serve the release
// snapshot instead of the hot draft.
//
// A field is released under version 1.0.0, then edited on the hot/draft
// project (a post-release change with no counterpart in the archive). An
// anonymous (non-editor) search for the released term must return that hit
// and must not see the later hot-only edit; an editor (explicit project
// owner role) keeps searching hot and does see it.
func TestSearch_ReleaseDefault_ServesReleasedSnapshot(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	pub := publication.NewReader(pool)
	h := NewHandler(store, slog.Default(), pub)
	r := chi.NewRouter()
	h.Mount(r)
	ctx := context.Background()

	ownerID := seedActor(t, pool, "TEST_SEARCH_RELDEF_OWNER")
	const projectID = "TEST_SEARCH_RELDEF_PROJECT"
	const fieldID = "TEST_SEARCH_RELDEF_FIELD"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_change_set WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_releases WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields_archive WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields WHERE id = $1`, fieldID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_projects (id, owner_id, visibility) VALUES ($1, $2, 'public')
	`, projectID, ownerID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_fields (
			id, semantic_id, system_name, ui_name, description, status, project_id
		) VALUES (
			$1, 'TEST.SEARCH.RELDEF.1', 'released-name', '{"en":"ReleasedOnlyTerm"}'::jsonb,
			'{"en":"released snapshot field"}'::jsonb, 'draft', $2
		)
	`, fieldID, projectID); err != nil {
		t.Fatalf("seed field: %v", err)
	}

	// Create a release — snapshots the live field (still "ReleasedOnlyTerm")
	// into weave_fields_archive under version 1.0.0.
	releaseCtx := weaveauth.WithPrincipal(
		weaveauth.WithSnapshot(ctx, &weaveauth.AuthSnapshot{IsSuperAdmin: true}),
		&weaveauth.Principal{ActorID: ownerID},
	)
	releaseSvc := release.NewService(nil, pool, nil)
	if _, err := releaseSvc.Create(releaseCtx, projectID, release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatalf("create release: %v", err)
	}

	// Mutate the live field post-release so hot diverges from the release
	// snapshot — a live-only edit with no counterpart in weave_fields_archive.
	if _, err := pool.Exec(ctx, `
		UPDATE weave_fields SET ui_name = '{"en":"HotOnlyTerm"}'::jsonb, system_name = 'hot-only-name'
		WHERE id = $1
	`, fieldID); err != nil {
		t.Fatalf("mutate live field: %v", err)
	}

	searchURL := func(q string) string {
		return "/api/v1/projects/" + projectID + "/search?type=field&q=" + q
	}

	t.Run("anonymous search for the released term returns the released hit, not a 404", func(t *testing.T) {
		items := doSearch(t, r, searchURL("ReleasedOnly"), nil)
		if len(items) != 1 || items[0].ID != fieldID {
			t.Fatalf("items=%+v, want the released field", items)
		}
		if items[0].UIName["en"] != "ReleasedOnlyTerm" {
			t.Fatalf("ui_name.en=%q, want the released name %q", items[0].UIName["en"], "ReleasedOnlyTerm")
		}
	})

	t.Run("anonymous search for the hot-only term returns nothing", func(t *testing.T) {
		items := doSearch(t, r, searchURL("HotOnly"), nil)
		if len(items) != 0 {
			t.Fatalf("items=%+v, want no hits — the hot-only edit must not leak into the release view", items)
		}
	})

	t.Run("editor search still hits hot and sees the live edit", func(t *testing.T) {
		snap := &weaveauth.AuthSnapshot{
			ActorID: "editor-actor",
			Roles:   map[string]string{"project:" + projectID: "owner"},
		}
		items := doSearch(t, r, searchURL("HotOnly"), snap)
		if len(items) != 1 || items[0].ID != fieldID {
			t.Fatalf("items=%+v, want the hot field", items)
		}
		if items[0].UIName["en"] != "HotOnlyTerm" {
			t.Fatalf("ui_name.en=%q, want the hot name %q", items[0].UIName["en"], "HotOnlyTerm")
		}
	})
}

func doSearch(t *testing.T, r chi.Router, target string, snap *weaveauth.AuthSnapshot) []domain.SearchResult {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if snap != nil {
		req = req.WithContext(weaveauth.WithSnapshot(req.Context(), snap))
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status=%d body=%s", target, rec.Code, rec.Body.String())
	}
	var resp domain.SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode search response: %v\n%s", err, rec.Body.String())
	}
	return resp.Items
}
