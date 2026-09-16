//go:build integration

package router_test

import (
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/i18n/backend"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/actorlabels"
	"github.com/pletka-io/pletka/pkg/weave/attribution"
	"github.com/pletka-io/pletka/pkg/weave/detailview"
	"github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/projectpage"
	"github.com/pletka-io/pletka/pkg/weave/publication"
	"github.com/pletka-io/pletka/pkg/weave/release"
	"github.com/pletka-io/pletka/pkg/weave/search"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// TestReleaseVisibilitySweep is Task 7's cross-surface regression sweep for
// the draft-vs-release visibility feature (Tasks 1-6). It mounts the real,
// production Mount functions of four slices — field (entity-list),
// detailview (entity detail), projectpage (page-schema), and search — each
// wrapped in exactly the middleware chain router.mountSlice/each slice's own
// Mount installs in production (auth.WithProjectVersionContext ->
// auth.WithProjectResource/RequireProjectRead -> auth.ResolveContentVersion),
// on one shared chi router, and asserts the combined public/private/
// no-release matrix end-to-end.
//
// Reproducing router.Mount's full ~30-field Options (every slice host, a
// live OntologyService, ...) by hand is explicitly out of scope — see
// mount_assembly_order_test.go's TestMount_RealEntrypointDoesNotHitChiAssemblyPanic,
// which documents that as impractical for a unit/integration test and uses a
// zero-value Options instead. This test takes the same approach the Task 4/
// 4b/6 integration tests already established (release_default_integration_test.go
// in detailview/search, entity_view_version_integration_test.go in
// detailview): mount each real slice's Mount function behind its production
// middleware chain, which is the part that actually matters for this
// feature. Each surface (entity-list, detail, page-schema, search) already
// has its own dedicated version-awareness test from an earlier task; this
// sweep's value is the combined public+release / private / public+no-release
// matrix exercised through one router.
func TestReleaseVisibilitySweep(t *testing.T) {
	pool := testdb.Pool(t)
	router := buildSweepRouter(t, pool)

	t.Run("public project with a release (hot != released)", func(t *testing.T) {
		testPublicProjectWithRelease(t, pool, router)
	})
	t.Run("private project with a member", func(t *testing.T) {
		testPrivateProjectWithMember(t, pool, router)
	})
	t.Run("public project with no release", func(t *testing.T) {
		testPublicProjectWithoutRelease(t, pool, router)
	})
}

// buildSweepRouter mounts the four named surfaces on one chi.Mux, each
// behind its production middleware chain, backed by real Postgres-backed
// stores/services against the shared test pool.
func buildSweepRouter(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()

	weaveStore := weave.NewPostgresStore(pool)
	fieldStore := field.NewPostgresStore(pool)
	pub := publication.NewReader(pool)

	router := chi.NewMux()

	// entity-list surface: field.Mount, wrapped in the same middleware chain
	// router.mountSlice installs for every project-scoped slice (see
	// router.go's mountSlice and detailview/release_default_integration_test.go,
	// which proves this exact wrapping end-to-end for field's Get-by-ID route).
	fieldSub := chi.NewMux()
	fieldSub.Use(auth.WithProjectVersionContext)
	fieldSub.Use(auth.WithProjectResource(weaveStore))
	fieldSub.Use(auth.ResolveContentVersion(pub))
	field.Mount(fieldSub, field.Host{
		Service:     field.NewService(fieldStore, nil, nil, nil, weaveStore.Projects(), nil, nil, nil, nil, nil),
		Projects:    weaveStore.Projects(),
		Publication: pub,
	})
	router.Mount("/projects/{projectID}/fields", fieldSub)

	// entity detail surface: detailview.Mount installs the equivalent chain
	// itself (auth.WithProjectVersionContext -> auth.WithProjectResource ->
	// auth.ResolveContentVersion), see detailview/handler.go Handler.Mount.
	i18nMgr := newSweepI18n(t)
	renderer := newSweepRenderer(t, i18nMgr)
	if err := detailview.Mount(router, detailview.Host{
		Weave:       weaveStore,
		I18n:        i18nMgr,
		Renderer:    renderer,
		Publication: pub,
		HasFormat:   func(generators.Format) bool { return false },
	}); err != nil {
		t.Fatalf("detailview.Mount: %v", err)
	}

	// project page-schema surface: projectpage.Mount installs its own
	// equivalent chain (auth.WithProjectVersionContext ->
	// auth.RequireProjectRead -> auth.ResolveContentVersion).
	projectpage.Mount(router, projectpage.Host{
		Logger:           slog.Default(),
		Weave:            weaveStore,
		LinkedOntologies: sweepLinkedOntologies{},
		Releases:         release.NewService(release.NewPostgresStore(pool), pool, slog.Default()),
		Attributions:     attribution.NewPostgresStore(pool),
		ActorLabels:      actorlabels.NewPostgresReader(pool),
		I18n:             i18nMgr,
		Publication:      pub,
	})

	// search surface: search.Mount installs its own equivalent chain
	// (auth.WithProjectVersionContext -> auth.RequireProjectRead ->
	// auth.ResolveContentVersion).
	search.Mount(router, search.Host{
		Weave:         weaveStore,
		Logger:        slog.Default(),
		LatestRelease: pub,
	})

	return router
}

// sweepLinkedOntologies is a no-op stand-in for projectpage.LinkedOntologyReader
// — the page-schema handler only needs the interface satisfied, not real
// ontology linkage data, to prove version threading.
type sweepLinkedOntologies struct{}

func (sweepLinkedOntologies) LinkedOntologies(context.Context, string) ([]domain.LinkedOntology, error) {
	return nil, nil
}

func newSweepI18n(t *testing.T) i18n.Manager {
	t.Helper()
	mgr, err := i18n.New(i18n.Config{
		DefaultLanguage:  "en",
		FallbackLanguage: "en",
		Storage:          backend.NewMemoryBackend(),
		Languages: []i18n.Language{
			{Code: "en", Name: "English", EnglishName: "English", Direction: "ltr", Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	return mgr
}

func newSweepRenderer(t *testing.T, i18nMgr i18n.Manager) *weavetemplates.Renderer {
	t.Helper()
	renderer, err := weavetemplates.NewRenderer(func(names ...string) template.HTML {
		return ""
	}, i18nMgr, weavetemplates.AnalyticsConfig{})
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}
	return renderer
}

// testPublicProjectWithRelease seeds a public project with one field,
// releases it under 1.0.0, then renames the live field so hot diverges from
// the release snapshot. Asserts: anonymous sees the released name on every
// surface, an org-inherited editor sees the hot name, and an explicit
// ?version= pins the released name regardless of viewer.
func testPublicProjectWithRelease(t *testing.T, pool *pgxpool.Pool, router http.Handler) {
	ctx := context.Background()
	const projectID = "RVSPUB"
	fieldID := ids.GenerateULID()

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_change_set WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_releases WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields_archive WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id=$1`, projectID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_projects (id, owner_id, visibility) VALUES ($1, 'unite', 'public')
	`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	fieldStore := field.NewPostgresStore(pool)
	seedField := &domain.Field{
		Entity: domain.Entity{
			ID:         fieldID,
			ProjectID:  projectID,
			SemanticID: "RVSPUBF.1",
			SystemName: "original_name",
			UIName:     domain.Translations{"en": "Original Name"},
			Status:     domain.StatusPublished,
		},
		PathElements: []domain.PathElement{},
	}
	if err := fieldStore.Create(ctx, seedField); err != nil {
		t.Fatalf("seed field: %v", err)
	}

	releaseCtx := auth.WithPrincipal(
		auth.WithSnapshot(ctx, &auth.AuthSnapshot{IsSuperAdmin: true}),
		&auth.Principal{ActorID: "unite"},
	)
	releaseSvc := release.NewService(nil, pool, nil)
	if _, err := releaseSvc.Create(releaseCtx, projectID, release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatalf("create release: %v", err)
	}

	// Mutate the live field post-release so hot diverges from the release
	// snapshot: entity-list/detail rename, search term drift.
	seedField.UIName = domain.Translations{"en": "Renamed Hot"}
	seedField.SystemName = "renamed_hot"
	if err := fieldStore.Update(ctx, seedField); err != nil {
		t.Fatalf("rename live field: %v", err)
	}

	editor := &auth.AuthSnapshot{
		ActorID: "org-inherited-owner",
		Roles:   map[string]string{"org:unite": "owner"},
	}

	listPath := "/projects/" + projectID + "/fields/"
	detailPath := "/projects/" + projectID + "/entity-view/field/" + fieldID
	schemaPath := "/projects/" + projectID + "/page-schema"
	searchReleased := "/api/v1/projects/" + projectID + "/search?type=field&q=Original"
	searchHotOnly := "/api/v1/projects/" + projectID + "/search?type=field&q=Renamed"

	t.Run("entity-list: anonymous sees the released name", func(t *testing.T) {
		code, resp := fetchFieldList(t, router, listPath, nil)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		if name, ok := fieldNameByID(resp, fieldID); !ok || name != "Original Name" {
			t.Fatalf("ui_name.en = %q (found=%v), want %q", name, ok, "Original Name")
		}
	})
	t.Run("entity-list: editor sees the hot name", func(t *testing.T) {
		code, resp := fetchFieldList(t, router, listPath, editor)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		if name, ok := fieldNameByID(resp, fieldID); !ok || name != "Renamed Hot" {
			t.Fatalf("ui_name.en = %q (found=%v), want %q", name, ok, "Renamed Hot")
		}
	})
	t.Run("entity-list: explicit version pins the released name", func(t *testing.T) {
		code, resp := fetchFieldList(t, router, listPath+"?version=1.0.0", nil)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		if name, ok := fieldNameByID(resp, fieldID); !ok || name != "Original Name" {
			t.Fatalf("ui_name.en = %q (found=%v), want %q", name, ok, "Original Name")
		}
	})

	t.Run("detail: anonymous sees the released name", func(t *testing.T) {
		code, name := fetchDetailName(t, router, detailPath, nil)
		if code != http.StatusOK || name != "Original Name" {
			t.Fatalf("status = %d, name = %q, want 200/%q", code, name, "Original Name")
		}
	})
	t.Run("detail: editor sees the hot name", func(t *testing.T) {
		code, name := fetchDetailName(t, router, detailPath, editor)
		if code != http.StatusOK || name != "Renamed Hot" {
			t.Fatalf("status = %d, name = %q, want 200/%q", code, name, "Renamed Hot")
		}
	})
	t.Run("detail: explicit version pins the released name", func(t *testing.T) {
		code, name := fetchDetailName(t, router, detailPath+"?version=1.0.0", nil)
		if code != http.StatusOK || name != "Original Name" {
			t.Fatalf("status = %d, name = %q, want 200/%q", code, name, "Original Name")
		}
	})

	t.Run("page-schema: anonymous overview URL is pinned to the release", func(t *testing.T) {
		code, url := fetchOverviewContentURL(t, router, schemaPath, nil)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		if !strings.Contains(url, "version=1.0.0") {
			t.Fatalf("overview content_url = %q, want it to carry version=1.0.0", url)
		}
	})
	t.Run("page-schema: editor overview URL stays version-less", func(t *testing.T) {
		code, url := fetchOverviewContentURL(t, router, schemaPath, editor)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		if strings.Contains(url, "version=") {
			t.Fatalf("overview content_url = %q, editor should not see a version pin", url)
		}
	})
	t.Run("page-schema: explicit version wins", func(t *testing.T) {
		code, url := fetchOverviewContentURL(t, router, schemaPath+"?version=1.0.0", nil)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		if !strings.Contains(url, "version=1.0.0") {
			t.Fatalf("overview content_url = %q, want it to carry version=1.0.0", url)
		}
	})

	t.Run("search: anonymous finds the released hit, not the hot-only edit", func(t *testing.T) {
		code, items := fetchSearch(t, router, searchReleased, nil)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		if len(items) != 1 || items[0].ID != fieldID {
			t.Fatalf("items = %+v, want the released field", items)
		}
		code2, items2 := fetchSearch(t, router, searchHotOnly, nil)
		if code2 != http.StatusOK {
			t.Fatalf("status = %d", code2)
		}
		if len(items2) != 0 {
			t.Fatalf("items = %+v, hot-only rename must not leak to an anonymous reader", items2)
		}
	})
	t.Run("search: editor finds the hot edit", func(t *testing.T) {
		code, items := fetchSearch(t, router, searchHotOnly, editor)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		if len(items) != 1 || items[0].ID != fieldID {
			t.Fatalf("items = %+v, want the hot field", items)
		}
	})
}

// testPrivateProjectWithMember seeds a private project with one member and
// one field. Asserts every surface still 404s an anonymous reader
// (unchanged existence-hiding behavior) and serves the hot content to the
// member — private projects never resolve to a release (see
// auth.ResolveEffectiveVersion: non-public always returns "").
func testPrivateProjectWithMember(t *testing.T, pool *pgxpool.Pool, router http.Handler) {
	ctx := context.Background()
	const projectID = "RVSPRIV"
	fieldID := ids.GenerateULID()

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id=$1`, projectID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_projects (id, owner_id, visibility) VALUES ($1, 'unite', 'private')
	`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	fieldStore := field.NewPostgresStore(pool)
	seedField := &domain.Field{
		Entity: domain.Entity{
			ID:         fieldID,
			ProjectID:  projectID,
			SemanticID: "RVSPRIVF.1",
			SystemName: "private_field",
			UIName:     domain.Translations{"en": "Private Field"},
			Status:     domain.StatusPublished,
		},
		PathElements: []domain.PathElement{},
	}
	if err := fieldStore.Create(ctx, seedField); err != nil {
		t.Fatalf("seed field: %v", err)
	}

	member := &auth.AuthSnapshot{
		ActorID: "private-member",
		Roles:   map[string]string{"project:" + projectID: "viewer"},
	}

	listPath := "/projects/" + projectID + "/fields/"
	detailPath := "/projects/" + projectID + "/entity-view/field/" + fieldID
	schemaPath := "/projects/" + projectID + "/page-schema"
	searchPath := "/api/v1/projects/" + projectID + "/search?type=field&q=Private"

	t.Run("entity-list: anonymous 404s, member sees hot", func(t *testing.T) {
		if code, _ := fetchFieldList(t, router, listPath, nil); code != http.StatusNotFound {
			t.Fatalf("anonymous status = %d, want 404", code)
		}
		code, resp := fetchFieldList(t, router, listPath, member)
		if code != http.StatusOK {
			t.Fatalf("member status = %d", code)
		}
		if name, ok := fieldNameByID(resp, fieldID); !ok || name != "Private Field" {
			t.Fatalf("ui_name.en = %q (found=%v), want %q", name, ok, "Private Field")
		}
	})

	t.Run("detail: anonymous 404s, member sees hot", func(t *testing.T) {
		if code, name := fetchDetailName(t, router, detailPath, nil); code != http.StatusNotFound {
			t.Fatalf("anonymous status = %d, name = %q, want 404", code, name)
		}
		code, name := fetchDetailName(t, router, detailPath, member)
		if code != http.StatusOK || name != "Private Field" {
			t.Fatalf("member status = %d, name = %q, want 200/%q", code, name, "Private Field")
		}
	})

	t.Run("page-schema: anonymous 404s, member reaches the page", func(t *testing.T) {
		if code, _ := fetchOverviewContentURL(t, router, schemaPath, nil); code != http.StatusNotFound {
			t.Fatalf("anonymous status = %d, want 404", code)
		}
		if code, _ := fetchOverviewContentURL(t, router, schemaPath, member); code != http.StatusOK {
			t.Fatalf("member status = %d", code)
		}
	})

	t.Run("search: anonymous 404s, member finds the hit", func(t *testing.T) {
		if code, _ := fetchSearch(t, router, searchPath, nil); code != http.StatusNotFound {
			t.Fatalf("anonymous status = %d, want 404", code)
		}
		code, items := fetchSearch(t, router, searchPath, member)
		if code != http.StatusOK {
			t.Fatalf("member status = %d", code)
		}
		if len(items) != 1 || items[0].ID != fieldID {
			t.Fatalf("items = %+v, want the private field", items)
		}
	})
}

// testPublicProjectWithoutRelease seeds a public project with no release at
// all. Asserts the status-quo fallback: an anonymous reader sees hot
// (there is no release to default to), and page-schema's overview URL
// carries no version pin.
func testPublicProjectWithoutRelease(t *testing.T, pool *pgxpool.Pool, router http.Handler) {
	ctx := context.Background()
	const projectID = "RVSNOREL"
	fieldID := ids.GenerateULID()

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id=$1`, projectID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_projects (id, owner_id, visibility) VALUES ($1, 'unite', 'public')
	`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	fieldStore := field.NewPostgresStore(pool)
	seedField := &domain.Field{
		Entity: domain.Entity{
			ID:         fieldID,
			ProjectID:  projectID,
			SemanticID: "RVSNORELF.1",
			SystemName: "no_release_field",
			UIName:     domain.Translations{"en": "NoRelease Field"},
			Status:     domain.StatusPublished,
		},
		PathElements: []domain.PathElement{},
	}
	if err := fieldStore.Create(ctx, seedField); err != nil {
		t.Fatalf("seed field: %v", err)
	}

	listPath := "/projects/" + projectID + "/fields/"
	detailPath := "/projects/" + projectID + "/entity-view/field/" + fieldID
	schemaPath := "/projects/" + projectID + "/page-schema"
	searchPath := "/api/v1/projects/" + projectID + "/search?type=field&q=NoRelease"

	t.Run("entity-list: anonymous falls back to hot", func(t *testing.T) {
		code, resp := fetchFieldList(t, router, listPath, nil)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		if name, ok := fieldNameByID(resp, fieldID); !ok || name != "NoRelease Field" {
			t.Fatalf("ui_name.en = %q (found=%v), want %q", name, ok, "NoRelease Field")
		}
	})

	t.Run("detail: anonymous falls back to hot", func(t *testing.T) {
		code, name := fetchDetailName(t, router, detailPath, nil)
		if code != http.StatusOK || name != "NoRelease Field" {
			t.Fatalf("status = %d, name = %q, want 200/%q", code, name, "NoRelease Field")
		}
	})

	t.Run("page-schema: anonymous overview URL carries no version pin", func(t *testing.T) {
		code, url := fetchOverviewContentURL(t, router, schemaPath, nil)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		if strings.Contains(url, "version=") {
			t.Fatalf("overview content_url = %q, want no version pin (no release to fall back to)", url)
		}
	})

	t.Run("search: anonymous finds the hot hit", func(t *testing.T) {
		code, items := fetchSearch(t, router, searchPath, nil)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		if len(items) != 1 || items[0].ID != fieldID {
			t.Fatalf("items = %+v, want the hot field", items)
		}
	})
}

// --- HTTP fetch helpers shared across the three fixtures above ---

type fieldListResponse struct {
	Fields []struct {
		ID     string            `json:"id"`
		UIName map[string]string `json:"ui_name"`
	} `json:"fields"`
}

func fieldNameByID(resp fieldListResponse, id string) (string, bool) {
	for _, f := range resp.Fields {
		if f.ID == id {
			return f.UIName["en"], true
		}
	}
	return "", false
}

func fetchFieldList(t *testing.T, router http.Handler, path string, snap *auth.AuthSnapshot) (int, fieldListResponse) {
	t.Helper()
	rec := doRequest(router, path, snap)
	var body fieldListResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode field list: %v\n%s", err, rec.Body.String())
		}
	}
	return rec.Code, body
}

func fetchDetailName(t *testing.T, router http.Handler, path string, snap *auth.AuthSnapshot) (int, string) {
	t.Helper()
	rec := doRequest(router, path, snap)
	if rec.Code != http.StatusOK {
		return rec.Code, ""
	}
	var body struct {
		Entity struct {
			Name map[string]string `json:"name"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode entity-view: %v\n%s", err, rec.Body.String())
	}
	return rec.Code, body.Entity.Name["en"]
}

func fetchOverviewContentURL(t *testing.T, router http.Handler, path string, snap *auth.AuthSnapshot) (int, string) {
	t.Helper()
	rec := doRequest(router, path, snap)
	if rec.Code != http.StatusOK {
		return rec.Code, ""
	}
	var body struct {
		Tabs []struct {
			ID         string `json:"id"`
			ContentURL string `json:"content_url"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode page-schema: %v\n%s", err, rec.Body.String())
	}
	for _, tab := range body.Tabs {
		if tab.ID == "overview" {
			return rec.Code, tab.ContentURL
		}
	}
	t.Fatalf("page-schema response has no overview tab: %s", rec.Body.String())
	return rec.Code, ""
}

func fetchSearch(t *testing.T, router http.Handler, path string, snap *auth.AuthSnapshot) (int, []domain.SearchResult) {
	t.Helper()
	rec := doRequest(router, path, snap)
	if rec.Code != http.StatusOK {
		return rec.Code, nil
	}
	var resp domain.SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode search response: %v\n%s", err, rec.Body.String())
	}
	return rec.Code, resp.Items
}

func doRequest(router http.Handler, path string, snap *auth.AuthSnapshot) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if snap != nil {
		req = req.WithContext(auth.WithSnapshot(req.Context(), snap))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
