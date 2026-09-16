//go:build integration

package detailview_test

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/i18n/backend"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/collection"
	"github.com/pletka-io/pletka/pkg/weave/detailview"
	"github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/model"
	"github.com/pletka-io/pletka/pkg/weave/override"
	"github.com/pletka-io/pletka/pkg/weave/publication"
	"github.com/pletka-io/pletka/pkg/weave/release"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// TestEntityViewVersion_ServesResolvedRelease proves Task 4b: detailview's
// entity-view builders (buildModel/buildCollection/buildField) load the
// entity HEADER (and buildField's override base) from the resolved
// release version when auth.ProjectVersionFromContext(ctx) is non-empty,
// instead of always reading hot. Before this task, buildModel/
// buildCollection/buildField called the unversioned GetByID/GetBase
// getters unconditionally, so a non-editor viewer of a public released
// project still saw the hot/draft header underneath already-versioned
// field/override composition (ModelView/CollectionView).
//
// Seeds a public project with a model, a collection, and a field (plus a
// base override on the field), takes a release snapshot, then renames all
// three live rows (and the override's set_value) so hot diverges from the
// release. Exercises the real entity-view route for each entity type,
// mounted behind the same middleware chain router.mountSlice installs
// (auth.WithProjectVersionContext -> auth.WithProjectResource ->
// auth.ResolveContentVersion), mirroring release_default_integration_test.go.
func TestEntityViewVersion_ServesResolvedRelease(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	const projectID = "ENTVER"
	modelID := ids.GenerateULID()
	collectionID := ids.GenerateULID()
	fieldID := ids.GenerateULID()

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_field_overrides_archive WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_field_overrides WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_change_set WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_releases WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_models_archive WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_collections_archive WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields_archive WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_models WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_collections WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_fields WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id=$1`, projectID)
	})

	// "unite" is a fixture organization actor seeded for every package's
	// testdb clone (internal/testdb/fixture_identities.go). Owning the
	// project with it lets us grant org-inherited edit access below.
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_projects (id, owner_id, visibility) VALUES ($1, 'unite', 'public')
	`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	fieldStore := field.NewPostgresStore(pool)
	modelStore := model.NewPostgresStore(pool)
	collectionStore := collection.NewPostgresStore(pool)
	overrideStore := override.NewPostgresStore(pool)
	weaveStore := weave.NewPostgresStore(pool)

	seedField := &domain.Field{
		Entity: domain.Entity{
			ID:         fieldID,
			ProjectID:  projectID,
			SemanticID: "ENTVERF.1",
			SystemName: "original_field",
			UIName:     domain.Translations{"en": "Original Field"},
			Status:     domain.StatusPublished,
		},
		// path_elements is jsonb NOT NULL — an explicit empty slice avoids
		// "cannot extract elements from a scalar" from the jsonb_array_elements
		// walk elsewhere.
		PathElements: []domain.PathElement{},
	}
	if err := fieldStore.Create(ctx, seedField); err != nil {
		t.Fatalf("seed field: %v", err)
	}

	seedModel := &domain.Model{
		Entity: domain.Entity{
			ID:         modelID,
			ProjectID:  projectID,
			SemanticID: "ENTVERM.1",
			SystemName: "original_model",
			UIName:     domain.Translations{"en": "Original Model"},
			Status:     domain.StatusPublished,
		},
		OntologyScope: domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E21_Person"},
		ModelType:     domain.ModelTypeAuxiliary,
	}
	if err := modelStore.Create(ctx, seedModel); err != nil {
		t.Fatalf("seed model: %v", err)
	}

	seedCollection := &domain.Collection{
		Entity: domain.Entity{
			ID:         collectionID,
			ProjectID:  projectID,
			SemanticID: "ENTVERC.1",
			SystemName: "original_collection",
			UIName:     domain.Translations{"en": "Original Collection"},
			Status:     domain.StatusPublished,
		},
		OntologyScope: domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E67_Birth"},
	}
	if err := collectionStore.Create(ctx, seedCollection); err != nil {
		t.Fatalf("seed collection: %v", err)
	}

	seedOverride := &domain.FieldOverride{
		FieldID:   fieldID,
		ProjectID: projectID,
		SetValue:  "original-value",
	}
	if err := overrideStore.Create(ctx, seedOverride); err != nil {
		t.Fatalf("seed base override: %v", err)
	}

	// Create a release — snapshots the live model/collection/field/override
	// rows (still "Original ...") into the *_archive tables under 1.0.0.
	releaseCtx := auth.WithPrincipal(
		auth.WithSnapshot(ctx, &auth.AuthSnapshot{IsSuperAdmin: true}),
		&auth.Principal{ActorID: "unite"},
	)
	releaseSvc := release.NewService(nil, pool, nil)
	if _, err := releaseSvc.Create(releaseCtx, projectID, release.CreateInput{Version: "1.0.0"}); err != nil {
		t.Fatalf("create release: %v", err)
	}

	// Mutate the live rows so hot diverges from the release snapshot.
	seedField.UIName = domain.Translations{"en": "Renamed Hot Field"}
	seedField.SystemName = "renamed_hot_field"
	if err := fieldStore.Update(ctx, seedField); err != nil {
		t.Fatalf("rename live field: %v", err)
	}
	seedModel.UIName = domain.Translations{"en": "Renamed Hot Model"}
	seedModel.SystemName = "renamed_hot_model"
	if err := modelStore.Update(ctx, seedModel); err != nil {
		t.Fatalf("rename live model: %v", err)
	}
	seedCollection.UIName = domain.Translations{"en": "Renamed Hot Collection"}
	seedCollection.SystemName = "renamed_hot_collection"
	if err := collectionStore.Update(ctx, seedCollection); err != nil {
		t.Fatalf("rename live collection: %v", err)
	}
	seedOverride.SetValue = "renamed-hot-value"
	if err := overrideStore.Update(ctx, seedOverride); err != nil {
		t.Fatalf("rename live override: %v", err)
	}

	i18nMgr, err := i18n.New(i18n.Config{
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
	renderer, err := weavetemplates.NewRenderer(func(names ...string) template.HTML {
		return ""
	}, i18nMgr, weavetemplates.AnalyticsConfig{})
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	host := detailview.Host{
		Weave:       weaveStore,
		I18n:        i18nMgr,
		Renderer:    renderer,
		Publication: publication.NewReader(pool),
		HasFormat:   func(generators.Format) bool { return false },
	}
	router := chi.NewMux()
	if err := detailview.Mount(router, host); err != nil {
		t.Fatalf("detailview.Mount: %v", err)
	}

	entityPath := func(entityType, entityID string) string {
		return "/projects/" + projectID + "/entity-view/" + entityType + "/" + entityID
	}

	orgOwner := &auth.AuthSnapshot{
		ActorID: "org-inherited-owner",
		Roles:   map[string]string{"org:unite": "owner"},
	}

	t.Run("model: anonymous sees the released header", func(t *testing.T) {
		name, _ := fetchEntity(t, router, entityPath("model", modelID), nil)
		if name != "Original Model" {
			t.Fatalf("entity.name.en = %q, want the released name %q", name, "Original Model")
		}
	})
	t.Run("model: editor sees the hot header", func(t *testing.T) {
		name, _ := fetchEntity(t, router, entityPath("model", modelID), orgOwner)
		if name != "Renamed Hot Model" {
			t.Fatalf("entity.name.en = %q, want the hot name %q", name, "Renamed Hot Model")
		}
	})
	t.Run("model: explicit version query serves the released header", func(t *testing.T) {
		name, _ := fetchEntity(t, router, entityPath("model", modelID)+"?version=1.0.0", nil)
		if name != "Original Model" {
			t.Fatalf("entity.name.en = %q, want the released name %q", name, "Original Model")
		}
	})

	t.Run("collection: anonymous sees the released header", func(t *testing.T) {
		name, _ := fetchEntity(t, router, entityPath("collection", collectionID), nil)
		if name != "Original Collection" {
			t.Fatalf("entity.name.en = %q, want the released name %q", name, "Original Collection")
		}
	})
	t.Run("collection: editor sees the hot header", func(t *testing.T) {
		name, _ := fetchEntity(t, router, entityPath("collection", collectionID), orgOwner)
		if name != "Renamed Hot Collection" {
			t.Fatalf("entity.name.en = %q, want the hot name %q", name, "Renamed Hot Collection")
		}
	})
	t.Run("collection: explicit version query serves the released header", func(t *testing.T) {
		name, _ := fetchEntity(t, router, entityPath("collection", collectionID)+"?version=1.0.0", nil)
		if name != "Original Collection" {
			t.Fatalf("entity.name.en = %q, want the released name %q", name, "Original Collection")
		}
	})

	t.Run("field: anonymous sees the released header and override base", func(t *testing.T) {
		name, setValue := fetchEntity(t, router, entityPath("field", fieldID), nil)
		if name != "Original Field" {
			t.Fatalf("entity.name.en = %q, want the released name %q", name, "Original Field")
		}
		if setValue != "original-value" {
			t.Fatalf("entity.set_value = %q, want the released override value %q", setValue, "original-value")
		}
	})
	t.Run("field: editor sees the hot header and override base", func(t *testing.T) {
		name, setValue := fetchEntity(t, router, entityPath("field", fieldID), orgOwner)
		if name != "Renamed Hot Field" {
			t.Fatalf("entity.name.en = %q, want the hot name %q", name, "Renamed Hot Field")
		}
		if setValue != "renamed-hot-value" {
			t.Fatalf("entity.set_value = %q, want the hot override value %q", setValue, "renamed-hot-value")
		}
	})
	t.Run("field: explicit version query serves the released header and override base", func(t *testing.T) {
		name, setValue := fetchEntity(t, router, entityPath("field", fieldID)+"?version=1.0.0", nil)
		if name != "Original Field" {
			t.Fatalf("entity.name.en = %q, want the released name %q", name, "Original Field")
		}
		if setValue != "original-value" {
			t.Fatalf("entity.set_value = %q, want the released override value %q", setValue, "original-value")
		}
	})
}

func fetchEntity(t *testing.T, router chi.Router, path string, snap *auth.AuthSnapshot) (name, setValue string) {
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
		Entity struct {
			Name     map[string]string `json:"name"`
			SetValue string             `json:"set_value"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal entity-view response: %v\n%s", err, rec.Body.String())
	}
	return body.Entity.Name["en"], body.Entity.SetValue
}
