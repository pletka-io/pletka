//go:build integration

package gitmaterializer_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/weave/category"
	"github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/model"
	"github.com/pletka-io/pletka/pkg/weave/override"
	"github.com/pletka-io/pletka/pkg/weave/project"
)

// roundTripNumberer is a minimal category.EntityNumberer that hands out
// monotonically increasing per-kind numbers. It mirrors the numberer
// production wiring passes to category.NewService so Create() allocates a
// real semantic ID instead of leaving it blank.
type roundTripNumberer struct {
	counts map[string]int64
}

func (n *roundTripNumberer) AllocateEntityNumber(_ context.Context, _, kind string) (int64, error) {
	n.counts[kind]++
	return n.counts[kind], nil
}

// countProjectRows returns the row count for table scoped to projectID.
// table is always one of the fixed literals passed by this test, never
// caller/user input.
func countProjectRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, projectID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE project_id = $1", projectID).Scan(&n); err != nil {
		t.Fatalf("count %s for %s: %v", table, projectID, err)
	}
	return n
}

// TestServerCreatedProjectRoundTrip proves the live-data-fallback
// requirement: a project whose content was authored the way a beta user
// would author it — through the category/field/model/override slice
// services and stores, never through an import tool — survives a full
// git-snapshot materialize + restore cycle.
//
// Construction layer per entity (documented per the task-6 brief):
//   - actor, project, ontology + ontology version: sqlcgen queries. These
//     are fixture prerequisites (matches the existing TestRestoreIntoEmptyDB
//     and TestHydrateProjectShell precedent); the project<->ontology-version
//     LINK itself is one of the round-tripped rows this test verifies.
//   - category: category.Service.Create — the real slice service, DI is
//     shallow enough (store + numberer) to wire directly in a test.
//   - field, model: field.Store / model.Store Create() calls directly.
//     field.Service and model.Service each carry ~8-10 constructor
//     dependencies (ProjectReader, HierarchyReader, OntologyRefRebuilder,
//     ViewReader, CategoryReader, ...) that only matter for HTTP-facing
//     business rules unrelated to this test; the store calls below
//     replicate exactly what Service.Create does (ID = SemanticID, status
//     defaulting) so the resulting rows are indistinguishable from
//     service-created ones.
//   - model override: override.Service.SaveForEntity — the real slice
//     service, DI is shallow (store + log + runner).
func TestServerCreatedProjectRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}

	ctx := context.Background()
	ctx = weaveauth.WithSnapshot(ctx, &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	pool := testPool(t)
	queries := sqlcgen.New(pool)

	const (
		projectID         = "T6RT"
		ontologyID        = "t6rt-ontology"
		ontologyVersionID = "t6rt-ontology-v1"
	)
	ownerID := "T6RT_OWNER"
	fieldID := projectID + "F.1"
	modelID := projectID + "M.1"

	t.Cleanup(func() {
		cleanupHydratedProject(t, pool, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE id = $1`, ontologyVersionID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, ontologyID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	// --- Fixture prerequisites: actor, project, ontology -------------------

	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          ownerID,
		Type:        "organization",
		DisplayName: "Round Trip Org",
		Slug:        "round-trip-org",
		Role:        "",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create owner actor: %v", err)
	}

	uiName, _ := json.Marshal(map[string]string{"en": "Round Trip Project"})
	description, _ := json.Marshal(map[string]string{"en": "Server-created round-trip fixture"})
	if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID:          projectID,
		UiName:      uiName,
		Description: description,
		Status:      "draft",
		OwnerID:     ownerID,
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	if _, err := queries.WeaveCreateOntology(ctx, sqlcgen.WeaveCreateOntologyParams{
		ID:           ontologyID,
		Prefix:       "rt",
		Namespace:    "https://example.org/round-trip/",
		Name:         "Round Trip Ontology",
		Description:  []byte(`{"en":"Round trip ontology"}`),
		OntologyType: "base",
		CreatedByID:  &ownerID,
	}); err != nil {
		t.Fatalf("create ontology: %v", err)
	}
	if _, err := queries.WeaveCreateOntologyVersion(ctx, sqlcgen.WeaveCreateOntologyVersionParams{
		ID:                     ontologyVersionID,
		OntologyID:             ontologyID,
		VersionString:          "1.0.0",
		IsActive:               true,
		CompatibleBaseVersions: []string{},
		RdfContent:             stringPtrRT("@prefix rt: <https://example.org/round-trip/> .\n"),
		ParsedAt:               pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		VersionInfo:            []byte(`{"en":"1.0.0"}`),
		ImportedOntologies:     []string{},
		OntologyLabel:          []byte(`{"en":"Round Trip Ontology"}`),
		OntologyComment:        []byte(`{"en":"Comment"}`),
		OntologyMetadata:       []byte(`{}`),
		ClassCount:             1,
		PropertyCount:          1,
	}); err != nil {
		t.Fatalf("create ontology version: %v", err)
	}

	isPrimary := true
	usageNotes := ""
	if _, err := queries.WeaveCreateProjectOntologyVersion(ctx, sqlcgen.WeaveCreateProjectOntologyVersionParams{
		ProjectID:         projectID,
		OntologyVersionID: ontologyVersionID,
		IsPrimary:         &isPrimary,
		UsageNotes:        &usageNotes,
	}); err != nil {
		t.Fatalf("link project to ontology version: %v", err)
	}

	// --- Server-created content: category, field, model, override ---------

	catStore := category.NewPostgresStore(pool)
	numberer := &roundTripNumberer{counts: map[string]int64{}}
	catSvc := category.NewService(catStore, nil, nil, nil, numberer)
	cat, err := catSvc.Create(ctx, projectID, category.CreateInput{
		SystemName:     "identity",
		UIName:         domain.Translations{"en": "Identity"},
		Description:    domain.Translations{"en": "Identity fields"},
		CanonicalOrder: 1,
	})
	if err != nil {
		t.Fatalf("category.Create: %v", err)
	}

	fieldStore := field.NewPostgresStore(pool)
	fld := &domain.Field{
		Entity: domain.Entity{
			ID:          fieldID,
			SemanticID:  fieldID,
			SystemName:  "name",
			UIName:      domain.Translations{"en": "Name"},
			Description: domain.Translations{"en": "Primary name"},
			Status:      domain.StatusPublished,
			ProjectID:   projectID,
		},
		OntologyScope: domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E21_Person"},
		PathElements: []domain.PathElement{
			{Type: "property", Prefix: "crm", LocalName: "P1_is_identified_by", Position: 0},
		},
		ExpectedValueType: "text",
	}
	if err := fieldStore.Create(ctx, fld); err != nil {
		t.Fatalf("field.Create: %v", err)
	}

	modelStore := model.NewPostgresStore(pool)
	mdl := &domain.Model{
		Entity: domain.Entity{
			ID:          modelID,
			SemanticID:  modelID,
			SystemName:  "person",
			UIName:      domain.Translations{"en": "Person"},
			Description: domain.Translations{"en": "Person model"},
			Status:      domain.StatusPublished,
			ProjectID:   projectID,
		},
		OntologyScope: domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E21_Person"},
		ModelType:     domain.ModelTypeAuxiliary,
	}
	if err := modelStore.Create(ctx, mdl); err != nil {
		t.Fatalf("model.Create: %v", err)
	}

	overrideStore := override.NewPostgresStore(pool)
	overrideSvc := override.NewService(overrideStore, nil, nil)
	savedOverrides, _, err := overrideSvc.SaveForEntity(ctx, projectID, "model", mdl.ID, []domain.FieldOverride{
		{
			FieldID:         fld.ID,
			Position:        1,
			CollectionOrder: 1,
			DisplayName:     domain.Translations{"en": "Full Name"},
			Description:     domain.Translations{"en": "Model-scoped override"},
			CategoryID:      cat.ID,
			IsRequired:      true,
			MinOccurs:       1,
			Visibility:      "visible",
		},
	}, "server-created round trip fixture")
	if err != nil {
		t.Fatalf("override.SaveForEntity: %v", err)
	}
	if len(savedOverrides) != 1 {
		t.Fatalf("expected 1 saved override, got %d", len(savedOverrides))
	}

	// --- Capture "before" state --------------------------------------------

	projectStore := project.NewPostgresStore(pool)
	projectBefore, err := projectStore.GetByID(ctx, projectID)
	if err != nil {
		t.Fatalf("get project before: %v", err)
	}
	if projectBefore == nil {
		t.Fatal("expected project to exist before materialize")
	}

	categoryCountBefore := countProjectRows(t, ctx, pool, "weave_categories", projectID)
	fieldCountBefore := countProjectRows(t, ctx, pool, "weave_fields", projectID)
	modelCountBefore := countProjectRows(t, ctx, pool, "weave_models", projectID)
	collectionCountBefore := countProjectRows(t, ctx, pool, "weave_collections", projectID)
	overrideCountBefore := countProjectRows(t, ctx, pool, "weave_field_overrides", projectID)

	overridesBefore, err := overrideSvc.ListForEntity(ctx, projectID, "model", mdl.ID)
	if err != nil {
		t.Fatalf("list overrides before: %v", err)
	}
	if len(overridesBefore) != 1 {
		t.Fatalf("expected 1 model override before, got %d", len(overridesBefore))
	}
	overrideBefore := overridesBefore[0]

	// --- Materialize to a git snapshot on disk ------------------------------

	sourceDir := t.TempDir()
	mat := gitmaterializer.NewMaterializer(pool, sourceDir, nil)
	if err := mat.InitProject(ctx, projectID); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	rootDir := filepath.Join(sourceDir, projectID)

	// --- Simulate an empty target database: scoped, FK-safe delete ---------

	cleanupHydratedProject(t, pool, projectID)

	if got, err := projectStore.GetByID(ctx, projectID); err != nil {
		t.Fatalf("get project after delete: %v", err)
	} else if got != nil {
		t.Fatalf("expected project to be deleted, still present: %#v", got)
	}

	// --- Restore from the on-disk snapshot ----------------------------------

	hydrator := gitmaterializer.NewMaterializer(pool, t.TempDir(), nil)
	if _, err := hydrator.HydrateProjectSnapshot(ctx, rootDir); err != nil {
		t.Fatalf("HydrateProjectSnapshot: %v", err)
	}

	// --- Assert project<->ontology-version link is restored ---------------

	links, err := queries.WeaveListProjectOntologyVersions(ctx, projectID)
	if err != nil {
		t.Fatalf("list project ontology versions after restore: %v", err)
	}
	if len(links) != 1 || links[0].OntologyVersionID != ontologyVersionID {
		t.Fatalf("expected project->ontology-version link to %s, got %#v", ontologyVersionID, links)
	}

	// --- Capture "after" state and compare -----------------------------

	projectAfter, err := projectStore.GetByID(ctx, projectID)
	if err != nil {
		t.Fatalf("get project after: %v", err)
	}
	if projectAfter == nil {
		t.Fatal("expected project to be restored")
	}

	entityOpts := cmp.Options{
		cmpopts.EquateEmpty(),
		cmpopts.IgnoreFields(domain.Entity{}, "CreatedAt", "UpdatedAt"),
	}
	if diff := cmp.Diff(projectBefore, projectAfter, entityOpts...); diff != "" {
		t.Fatalf("restored project mismatch (-before +after):\n%s", diff)
	}

	if got := countProjectRows(t, ctx, pool, "weave_categories", projectID); got != categoryCountBefore {
		t.Fatalf("category count mismatch: before=%d after=%d", categoryCountBefore, got)
	}
	if got := countProjectRows(t, ctx, pool, "weave_fields", projectID); got != fieldCountBefore {
		t.Fatalf("field count mismatch: before=%d after=%d", fieldCountBefore, got)
	}
	if got := countProjectRows(t, ctx, pool, "weave_models", projectID); got != modelCountBefore {
		t.Fatalf("model count mismatch: before=%d after=%d", modelCountBefore, got)
	}
	if got := countProjectRows(t, ctx, pool, "weave_collections", projectID); got != collectionCountBefore {
		t.Fatalf("collection count mismatch: before=%d after=%d", collectionCountBefore, got)
	}
	if got := countProjectRows(t, ctx, pool, "weave_field_overrides", projectID); got != overrideCountBefore {
		t.Fatalf("override count mismatch: before=%d after=%d", overrideCountBefore, got)
	}

	overridesAfter, err := overrideSvc.ListForEntity(ctx, projectID, "model", modelID)
	if err != nil {
		t.Fatalf("list overrides after: %v", err)
	}
	if len(overridesAfter) != 1 {
		t.Fatalf("expected 1 model override after restore, got %d", len(overridesAfter))
	}
	overrideAfter := overridesAfter[0]

	overrideRowOpts := cmp.Options{
		cmpopts.EquateEmpty(),
		cmpopts.IgnoreFields(domain.FieldOverride{}, "ID", "FieldID", "CategoryID", "CreatedAt", "UpdatedAt"),
	}
	if diff := cmp.Diff(overrideBefore, overrideAfter, overrideRowOpts...); diff != "" {
		t.Fatalf("restored model override mismatch (-before +after):\n%s", diff)
	}
	// FieldID is intentionally excluded from the struct diff above: field
	// identity is regenerated by the restore path (a fresh ULID replaces
	// the server-created field's SemanticID-as-ID — see hydrateFieldFile
	// in restore_entities.go), so the override's FieldID legitimately
	// differs before/after. It is re-resolved via SemanticID at restore
	// time (resolveFieldIdentifier in restore_overrides.go), so the
	// reference itself must still point at a real, live field row.
	// Assert that resolution explicitly instead of a literal FieldID
	// comparison.
	restoredField, err := fieldStore.GetByIdentifier(ctx, projectID, fieldID)
	if err != nil {
		t.Fatalf("get restored field by semantic id: %v", err)
	}
	if restoredField == nil {
		t.Fatal("expected restored field to exist")
	}
	if overrideAfter.FieldID != restoredField.ID {
		t.Fatalf("expected restored override FieldID %q to match restored field ID %q", overrideAfter.FieldID, restoredField.ID)
	}
	if overrideAfter.FieldID == overrideBefore.FieldID {
		t.Fatalf("expected restored override FieldID to be regenerated, still %q", overrideAfter.FieldID)
	}
	// CategoryID is intentionally excluded from the struct diff above for the
	// same reason as FieldID: categories are always re-created with a fresh
	// ULID during restore (hydrateCategoryFile in restore_entities.go), so
	// the override's CategoryID legitimately differs before/after. Without
	// this explicit resolution check, a regression that copies category_id
	// verbatim from the snapshot (a stale, now-dangling ULID) would slip
	// through unnoticed: the literal string round-trips unchanged and a
	// plain struct diff can't tell "same value" from "still resolves".
	restoredCategory, err := catStore.GetByIdentifier(ctx, projectID, cat.SemanticID)
	if err != nil {
		t.Fatalf("get restored category by semantic id: %v", err)
	}
	if restoredCategory == nil {
		t.Fatal("expected restored category to exist")
	}
	if overrideAfter.CategoryID != restoredCategory.ID {
		t.Fatalf("expected restored override CategoryID %q to match restored category ID %q", overrideAfter.CategoryID, restoredCategory.ID)
	}
	if overrideAfter.CategoryID == overrideBefore.CategoryID {
		t.Fatalf("expected restored override CategoryID to be regenerated, still %q", overrideAfter.CategoryID)
	}

	// --- Second-run idempotency: restore again and verify no duplicates ----

	if _, err := hydrator.HydrateProjectSnapshot(ctx, rootDir); err != nil {
		t.Fatalf("HydrateProjectSnapshot (second run): %v", err)
	}

	linksAfterSecondRun, err := queries.WeaveListProjectOntologyVersions(ctx, projectID)
	if err != nil {
		t.Fatalf("list project ontology versions after second run: %v", err)
	}
	if len(linksAfterSecondRun) != 1 || linksAfterSecondRun[0].OntologyVersionID != ontologyVersionID {
		t.Fatalf("expected project->ontology-version link to remain %s after second run (idempotent), got %#v", ontologyVersionID, linksAfterSecondRun)
	}
}

func stringPtrRT(v string) *string {
	return &v
}
