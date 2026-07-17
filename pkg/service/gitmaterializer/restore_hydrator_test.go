package gitmaterializer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func hydrateTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:pw123@localhost:5433/pletka_weave?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("database not reachable: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestHydrateProjectShell(t *testing.T) {
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)

	ownerID := "RESTORE_OWNER_ACTOR"
	creatorID := "RESTORE_CREATOR_ACTOR"
	projectID := "RESTORE_CHILD"
	parentA := "RESTORE_PARENT_A"
	parentB := "RESTORE_PARENT_B"
	ontologyID := "restore-ontology"
	ontologyVersionID := "restore-ontology-v1"

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE project_id = ANY($1)`, []string{projectID, parentA, parentB})
		_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = ANY($1)`, []string{projectID, parentA, parentB})
		_, _ = pool.Exec(ctx, `DELETE FROM weave_project_inheritance WHERE project_id = ANY($1) OR parent_project_id = ANY($1)`, []string{projectID, parentA, parentB})
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = ANY($1)`, []string{projectID, parentA, parentB})
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE id = $1`, ontologyVersionID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, ontologyID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = ANY($1)`, []string{ownerID, creatorID})
	})

	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          ownerID,
		Type:        "organization",
		DisplayName: "Restore Org",
		Slug:        "restore-org",
		Role:        "",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create owner actor: %v", err)
	}
	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          creatorID,
		Type:        "person",
		DisplayName: "Restore Curator",
		Slug:        "restore-curator",
		Role:        "",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create creator actor: %v", err)
	}

	uiNameParent, _ := json.Marshal(map[string]string{"en": "Parent"})
	descParent, _ := json.Marshal(map[string]string{"en": "Parent project"})
	for _, id := range []string{parentA, parentB} {
		if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
			ID:          id,
			UiName:      uiNameParent,
			Description: descParent,
			Status:      "draft",
			OwnerID:     ownerID,
			Visibility:  "private",
		}); err != nil {
			t.Fatalf("create parent project %s: %v", id, err)
		}
	}

	if _, err := queries.WeaveCreateOntology(ctx, sqlcgen.WeaveCreateOntologyParams{
		ID:           ontologyID,
		Prefix:       "ro",
		Namespace:    "https://example.org/restore/",
		Name:         "Restore Ontology",
		Description:  []byte(`{"en":"Restore ontology"}`),
		OntologyType: "base",
		CreatedByID:  &creatorID,
	}); err != nil {
		t.Fatalf("create ontology: %v", err)
	}
	if _, err := queries.WeaveCreateOntologyVersion(ctx, sqlcgen.WeaveCreateOntologyVersionParams{
		ID:                     ontologyVersionID,
		OntologyID:             ontologyID,
		VersionString:          "1.0.0",
		IsActive:               true,
		CompatibleBaseVersions: []string{},
		RdfContent:             stringPtr("@prefix ro: <https://example.org/restore/> .\n"),
		ParsedAt:               pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		VersionInfo:            []byte(`{"en":"1.0.0"}`),
		ImportedOntologies:     []string{},
		OntologyLabel:          []byte(`{"en":"Restore Ontology"}`),
		OntologyComment:        []byte(`{"en":"Comment"}`),
		OntologyMetadata:       []byte(`{}`),
		ClassCount:             1,
		PropertyCount:          1,
	}); err != nil {
		t.Fatalf("create ontology version: %v", err)
	}

	snapshot := &ProjectSnapshot{
		Manifest: ProjectManifestFile{
			SchemaVersion: 1,
			Project: ProjectManifestProject{
				ID:          projectID,
				Title:       map[string]string{"en": "Hydrated Child"},
				Description: map[string]string{"en": "Hydrated description"},
				Readme:      map[string]string{"en": "Hydrated readme"},
				Namespace:   "https://example.org/hydrated/",
				Visibility:  "private",
				Owner: &manifestActor{
					ActorID:     ownerID,
					Type:        "organization",
					Slug:        "restore-org",
					DisplayName: "Restore Org",
				},
				CreatedBy: &manifestActor{
					ActorID:     creatorID,
					Type:        "person",
					Slug:        "restore-curator",
					DisplayName: "Restore Curator",
				},
				License: "CC-BY-4.0",
				BaseURL: "https://example.org/data/",
				Topics:  []string{"restored", "git"},
			},
			Inheritance: &ProjectManifestInheritance{
				Parents: []ProjectManifestParent{
					{ProjectID: parentA, IsPrimary: false, CanonicalOrder: 1, SourceMode: "draft"},
					{ProjectID: parentB, IsPrimary: true, CanonicalOrder: 0, SourceMode: "release", SourceVersion: "2.0.0"},
				},
			},
			Ontologies: &ProjectManifestOntologies{
				LinkedVersions: []ProjectManifestLinkedOntology{
					{
						OntologyID:        ontologyID,
						OntologyVersionID: ontologyVersionID,
						Version:           "1.0.0",
						IsPrimary:         true,
					},
				},
			},
			Namespaces: &ProjectManifestNamespaces{
				EffectiveBindings: []ProjectManifestNamespaceBinding{
					{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/", Source: "system"},
					{Prefix: "ro", Namespace: "https://example.org/restore/", Source: "ontology"},
				},
			},
			Manifests: &ProjectManifestManifests{},
		},
		Mod: &PletkaModFile{
			SchemaVersion: 1,
			Module: PletkaModModule{
				Path:      "pletka.io/orgs/restore-org/projects/RESTORE_CHILD",
				ProjectID: projectID,
			},
		},
	}
	plan := BuildRestorePlan(snapshot)

	mat := NewMaterializer(pool, t.TempDir(), nil)
	if err := mat.HydrateProjectShell(ctx, plan); err != nil {
		t.Fatalf("HydrateProjectShell: %v", err)
	}

	project, err := queries.WeaveGetProjectByID(ctx, projectID)
	if err != nil {
		t.Fatalf("get hydrated project: %v", err)
	}
	if project.OwnerID != ownerID {
		t.Fatalf("expected owner %s, got %s", ownerID, project.OwnerID)
	}
	if project.CreatedByID == nil || *project.CreatedByID != creatorID {
		t.Fatalf("expected created_by %s, got %#v", creatorID, project.CreatedByID)
	}
	if project.ParentProjectID == nil || *project.ParentProjectID != parentB {
		t.Fatalf("expected primary parent %s, got %#v", parentB, project.ParentProjectID)
	}
	if project.License != "CC-BY-4.0" || project.BaseUrl != "https://example.org/data/" {
		t.Fatalf("expected about fields to hydrate, got license=%q base_url=%q", project.License, project.BaseUrl)
	}

	inheritanceRows, err := pool.Query(ctx, `
		SELECT parent_project_id, is_primary, canonical_order, source_mode, COALESCE(source_version, '')
		FROM weave_project_inheritance
		WHERE project_id = $1
		ORDER BY canonical_order ASC, parent_project_id ASC
	`, projectID)
	if err != nil {
		t.Fatalf("query inheritances: %v", err)
	}
	defer inheritanceRows.Close()
	type inheritanceRow struct {
		parentID string
		primary  bool
		order    int32
		mode     string
		version  string
	}
	var inheritances []inheritanceRow
	for inheritanceRows.Next() {
		var row inheritanceRow
		if err := inheritanceRows.Scan(&row.parentID, &row.primary, &row.order, &row.mode, &row.version); err != nil {
			t.Fatalf("scan inheritance: %v", err)
		}
		inheritances = append(inheritances, row)
	}
	if len(inheritances) != 2 {
		t.Fatalf("expected 2 inheritances, got %#v", inheritances)
	}
	if !inheritances[0].primary || inheritances[0].parentID != parentB {
		t.Fatalf("expected primary inheritance to %s, got %#v", parentB, inheritances)
	}
	if inheritances[0].mode != "release" || inheritances[0].version != "2.0.0" {
		t.Fatalf("expected pinned primary inheritance, got %#v", inheritances[0])
	}
	if inheritances[1].mode != "draft" || inheritances[1].version != "" {
		t.Fatalf("expected draft secondary inheritance, got %#v", inheritances[1])
	}

	links, err := queries.WeaveListProjectOntologyVersions(ctx, projectID)
	if err != nil {
		t.Fatalf("list ontology links: %v", err)
	}
	if len(links) != 1 || links[0].OntologyVersionID != ontologyVersionID || !boolValue(links[0].IsPrimary) {
		t.Fatalf("unexpected ontology links: %#v", links)
	}

	bindings, err := pool.Query(ctx, `
		SELECT prefix, namespace, weight, source
		FROM weave_namespace_bindings
		WHERE project_id = $1
		ORDER BY weight ASC, prefix ASC
	`, projectID)
	if err != nil {
		t.Fatalf("query namespace bindings: %v", err)
	}
	defer bindings.Close()
	var gotBindings []struct {
		prefix    string
		namespace string
		weight    int64
		source    string
	}
	for bindings.Next() {
		var row struct {
			prefix    string
			namespace string
			weight    int64
			source    string
		}
		if err := bindings.Scan(&row.prefix, &row.namespace, &row.weight, &row.source); err != nil {
			t.Fatalf("scan namespace binding: %v", err)
		}
		gotBindings = append(gotBindings, row)
	}
	if len(gotBindings) != 1 {
		t.Fatalf("expected 1 project-owned namespace binding, got %#v", gotBindings)
	}
	if gotBindings[0].source != "user" {
		t.Fatalf("expected user namespace bindings, got %#v", gotBindings)
	}
	if gotBindings[0].prefix != "ro" {
		t.Fatalf("expected project-owned ro binding, got %#v", gotBindings)
	}
}

func boolValue(v *bool) bool {
	return v != nil && *v
}

func TestHydrateProjectSnapshot(t *testing.T) {
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)

	ownerID := "RESTORE_SNAPSHOT_OWNER"
	creatorID := "RESTORE_SNAPSHOT_CREATOR"
	projectID := "RESTORE_SNAPSHOT_CHILD"
	root := t.TempDir()

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_project_inheritance WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = ANY($1)`, []string{ownerID, creatorID})
	})

	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          ownerID,
		Type:        "organization",
		DisplayName: "Restore Snapshot Org",
		Slug:        "restore-snapshot-org",
		Role:        "",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create owner actor: %v", err)
	}
	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          creatorID,
		Type:        "person",
		DisplayName: "Restore Snapshot Curator",
		Slug:        "restore-snapshot-curator",
		Role:        "",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create creator actor: %v", err)
	}

	projectManifestPayload, err := encodeProjectManifest(projectManifest{
		SchemaVersion: 1,
		Project: projectManifestProject{
			ID:          projectID,
			Title:       map[string]string{"en": "Hydrated Snapshot"},
			Description: map[string]string{"en": "Hydrated from disk"},
			Readme:      map[string]string{"en": "Round-trip restore"},
			Namespace:   "https://example.org/restore-snapshot/",
			Visibility:  "private",
			Owner: &manifestActor{
				ActorID:     ownerID,
				Type:        "organization",
				Slug:        "restore-snapshot-org",
				DisplayName: "Restore Snapshot Org",
			},
			CreatedBy: &manifestActor{
				ActorID:     creatorID,
				Type:        "person",
				Slug:        "restore-snapshot-curator",
				DisplayName: "Restore Snapshot Curator",
			},
			License: "CC-BY-4.0",
			BaseURL: "https://example.org/data/",
			Topics:  []string{"restore", "snapshot"},
		},
		Manifests: &projectManifestManifests{
			Adoptions: "adoptions/index.yaml",
			Forks:     "forks/index.yaml",
		},
	})
	if err != nil {
		t.Fatalf("encode project manifest: %v", err)
	}
	if err := writeEntityFile(root, "project.yaml", projectManifestPayload); err != nil {
		t.Fatalf("write project manifest: %v", err)
	}

	modPayload, err := encodePletkaModManifest(pletkaModManifest{
		SchemaVersion: 1,
		Module: pletkaModModule{
			Path:      "pletka.io/orgs/restore-snapshot-org/projects/RESTORE_SNAPSHOT_CHILD",
			ProjectID: projectID,
		},
		Require: pletkaModRequire{
			Projects:   []pletkaProjectRequirement{},
			Ontologies: []pletkaOntologyRequirement{},
		},
		Replace: []map[string]any{},
	})
	if err != nil {
		t.Fatalf("encode pletka.mod: %v", err)
	}
	if err := writeEntityFile(root, "pletka.mod", modPayload); err != nil {
		t.Fatalf("write pletka.mod: %v", err)
	}

	adoptionsIndexPayload, err := encodeAdoptionsIndexManifest(adoptionsIndexManifest{
		SchemaVersion: 1,
		Receipts:      []adoptionsIndexReceiptEntry{},
	})
	if err != nil {
		t.Fatalf("encode empty adoptions index: %v", err)
	}
	if err := writeEntityFile(root, "adoptions/index.yaml", adoptionsIndexPayload); err != nil {
		t.Fatalf("write adoptions index: %v", err)
	}

	forksIndexPayload, err := encodeForksIndexManifest(forksIndexManifest{
		SchemaVersion: 1,
		Receipts:      []forksIndexReceiptEntry{},
	})
	if err != nil {
		t.Fatalf("encode empty forks index: %v", err)
	}
	if err := writeEntityFile(root, "forks/index.yaml", forksIndexPayload); err != nil {
		t.Fatalf("write forks index: %v", err)
	}

	mat := NewMaterializer(pool, t.TempDir(), nil)
	plan, err := mat.HydrateProjectSnapshot(ctx, root)
	if err != nil {
		t.Fatalf("HydrateProjectSnapshot: %v", err)
	}
	if plan.ProjectID != projectID {
		t.Fatalf("expected restore plan project %s, got %s", projectID, plan.ProjectID)
	}

	project, err := queries.WeaveGetProjectByID(ctx, projectID)
	if err != nil {
		t.Fatalf("get hydrated project: %v", err)
	}
	if project.OwnerID != ownerID {
		t.Fatalf("expected owner %s, got %s", ownerID, project.OwnerID)
	}
	if project.CreatedByID == nil || *project.CreatedByID != creatorID {
		t.Fatalf("expected created_by %s, got %#v", creatorID, project.CreatedByID)
	}
}

func TestHydrateProjectEntities(t *testing.T) {
	ctx := context.Background()
	pool := hydrateTestPool(t)
	queries := sqlcgen.New(pool)

	ownerID := "RESTORE_ENTITY_OWNER"
	projectID := "RESTORE_ENTITY_CHILD"

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_categories WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_collections WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_namespace_bindings WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_project_inheritance WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          ownerID,
		Type:        "organization",
		DisplayName: "Restore Entity Org",
		Slug:        "restore-entity-org",
		Role:        "",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create owner actor: %v", err)
	}

	snapshot := &ProjectSnapshot{
		Manifest: ProjectManifestFile{
			SchemaVersion: 1,
			Project: ProjectManifestProject{
				ID:         projectID,
				Title:      map[string]string{"en": "Entity Child"},
				Namespace:  "https://example.org/entity/",
				Visibility: "private",
				Owner: &manifestActor{
					ActorID:     ownerID,
					Type:        "organization",
					Slug:        "restore-entity-org",
					DisplayName: "Restore Entity Org",
				},
			},
			Manifests: &ProjectManifestManifests{},
		},
		Mod: &PletkaModFile{
			SchemaVersion: 1,
			Module: PletkaModModule{
				Path:      "pletka.io/orgs/restore-entity-org/projects/RESTORE_ENTITY_CHILD",
				ProjectID: projectID,
			},
		},
	}

	writeCanonical := func(t *testing.T, payload []byte, path, entityType, entityID string) SnapshotEntityFile {
		t.Helper()
		return SnapshotEntityFile{
			Path:       path,
			EntityType: entityType,
			EntityID:   entityID,
			Payload:    payload,
		}
	}

	categoryPayload, err := canonical.Category(&domain.Category{
		Entity: domain.Entity{
			SemanticID:  "RESTORE.CAT.1",
			SystemName:  "identity",
			UIName:      domain.Translations{"en": "Identity"},
			Description: domain.Translations{"en": "Identity fields"},
			Status:      "draft",
		},
		CanonicalOrder: 2,
	})
	if err != nil {
		t.Fatalf("encode category: %v", err)
	}
	fieldPayload, err := canonical.Field(&domain.Field{
		Entity: domain.Entity{
			SemanticID:  "RESTOREF.1",
			SystemName:  "name",
			UIName:      domain.Translations{"en": "Name"},
			Description: domain.Translations{"en": "Primary name"},
			Status:      "published",
		},
		OntologyScope: domain.PathElement{Prefix: "crm", LocalName: "E21_Person"},
		PathElements: []domain.PathElement{
			{Prefix: "crm", LocalName: "P1_is_identified_by", Position: 0},
			{Prefix: "rdfs", LocalName: "Literal", Position: 1},
		},
		ExpectedValueType: "text",
	})
	if err != nil {
		t.Fatalf("encode field: %v", err)
	}
	modelPayload, err := canonical.Model(&domain.Model{
		Entity: domain.Entity{
			SemanticID:  "RESTOREM.1",
			SystemName:  "person",
			UIName:      domain.Translations{"en": "Person"},
			Description: domain.Translations{"en": "Person model"},
			Status:      "published",
		},
		OntologyScope: domain.PathElement{Prefix: "crm", LocalName: "E21_Person"},
	})
	if err != nil {
		t.Fatalf("encode model: %v", err)
	}
	collectionPayload, err := canonical.Collection(&domain.Collection{
		Entity: domain.Entity{
			SemanticID:  "RESTOREC.1",
			SystemName:  "birth",
			UIName:      domain.Translations{"en": "Birth"},
			Description: domain.Translations{"en": "Birth collection"},
			Status:      "draft",
			Deprecated:  true,
		},
		OntologyScope:            domain.PathElement{Prefix: "crm", LocalName: "E67_Birth"},
		CollectionNumber:         3,
		CanonicalCollectionOrder: 4,
	})
	if err != nil {
		t.Fatalf("encode collection: %v", err)
	}
	adoptedModelPayload, err := canonical.Model(&domain.Model{
		Entity: domain.Entity{
			SemanticID:  "LAM.13",
			SystemName:  "upstream-person",
			UIName:      domain.Translations{"en": "Upstream Person"},
			Description: domain.Translations{"en": "Upstream"},
			Status:      "published",
		},
		OntologyScope: domain.PathElement{Prefix: "crm", LocalName: "E21_Person"},
	})
	if err != nil {
		t.Fatalf("encode adopted model: %v", err)
	}
	adoptedModelPayload, err = addAdoptionMarker(adoptedModelPayload, "LA")
	if err != nil {
		t.Fatalf("annotate adopted model: %v", err)
	}

	snapshot.Entities = SnapshotEntityTree{
		Categories: []SnapshotEntityFile{
			writeCanonical(t, categoryPayload, "categories/RESTORE.CAT.1.yaml", "category", "RESTORE.CAT.1"),
		},
		Fields: []SnapshotEntityFile{
			writeCanonical(t, fieldPayload, "fields/RESTOREF.1/field.yaml", "field", "RESTOREF.1"),
		},
		Models: []SnapshotEntityFile{
			writeCanonical(t, modelPayload, "models/RESTOREM.1/model.yaml", "model", "RESTOREM.1"),
			writeCanonical(t, adoptedModelPayload, "models/LAM.13/model.yaml", "model", "LAM.13"),
		},
		Collections: []SnapshotEntityFile{
			writeCanonical(t, collectionPayload, "collections/RESTOREC.1/collection.yaml", "collection", "RESTOREC.1"),
		},
	}
	baseOverridePayload, err := canonical.Override(&domain.FieldOverride{
		FieldID:         "RESTOREF.1",
		Position:        1,
		DisplayName:     domain.Translations{"en": "Primary Name"},
		Description:     domain.Translations{"en": "Base field override"},
		CategoryID:      "RESTORE.CAT.1",
		IsRequired:      true,
		MinOccurs:       1,
		CollectionOrder: 0,
	}, nil)
	if err != nil {
		t.Fatalf("encode base override: %v", err)
	}
	modelOverridePayload, err := canonical.Override(&domain.FieldOverride{
		FieldID:         "RESTOREF.1",
		EntityType:      "model",
		EntityID:        "RESTOREM.1",
		Position:        2,
		DisplayName:     domain.Translations{"en": "Model Name"},
		CategoryID:      "RESTORE.CAT.1",
		CollectionOrder: 3,
	}, []domain.OverrideRef{
		{RefType: "resource_model", SemanticID: "RESTOREM.1", Position: 0},
		{RefType: "collection_model", SemanticID: "RESTOREC.1", Position: 1},
	})
	if err != nil {
		t.Fatalf("encode model override: %v", err)
	}
	collectionOverridePayload, err := canonical.Override(&domain.FieldOverride{
		FieldID:         "RESTOREF.1",
		EntityType:      "collection",
		EntityID:        "RESTOREC.1",
		Position:        4,
		SetValue:        "fixed",
		CollectionOrder: 2,
		IsHidden:        true,
	}, nil)
	if err != nil {
		t.Fatalf("encode collection override: %v", err)
	}
	snapshot.Entities.BaseOverrides = []SnapshotEntityFile{
		{
			Path:       "fields/RESTOREF.1/base-override.yaml",
			EntityType: "base_override",
			FieldID:    "RESTOREF.1",
			Payload:    baseOverridePayload,
		},
	}
	snapshot.Entities.ModelOverrides = []SnapshotEntityFile{
		{
			Path:       "models/RESTOREM.1/overrides/RESTOREF.1@1.yaml",
			EntityType: "model_override",
			FieldID:    "RESTOREF.1",
			OwnerType:  "model",
			OwnerID:    "RESTOREM.1",
			EntityID:   "1",
			Payload:    modelOverridePayload,
		},
	}
	snapshot.Entities.CollectionOverrides = []SnapshotEntityFile{
		{
			Path:       "collections/RESTOREC.1/overrides/RESTOREF.1@1.yaml",
			EntityType: "collection_override",
			FieldID:    "RESTOREF.1",
			OwnerType:  "collection",
			OwnerID:    "RESTOREC.1",
			EntityID:   "1",
			Payload:    collectionOverridePayload,
		},
	}
	snapshot.Adoptions = &AdoptionManifestSet{
		Index: AdoptionsIndexManifest{
			SchemaVersion: 1,
			Receipts: []AdoptionsIndexReceiptEntry{
				{EntityType: "model", EntityID: "LAM.13", File: "model-LAM.13.yaml"},
			},
		},
		Receipts: map[string]AdoptionReceiptManifest{
			"model-LAM.13.yaml": {
				SchemaVersion: 1,
				Adoption: AdoptionReceiptRecord{
					EntityType: "model",
					EntityID:   "LAM.13",
					Source: AdoptionReceiptSource{
						ProjectID: "LA",
						EntityID:  "LAM.13",
						Version:   "1.0.0",
					},
					Contexts: []AdoptionReceiptContext{
						{
							ContextEntityType: "project",
							ContextEntityID:   projectID,
						},
					},
				},
			},
		},
	}
	snapshot.Forks = &ForkManifestSet{
		Index: ForksIndexManifest{
			SchemaVersion: 1,
			Receipts: []ForksIndexReceiptEntry{
				{EntityType: "collection", EntityID: "RESTOREC.1", File: "collection-RESTOREC.1.yaml"},
			},
		},
		Receipts: map[string]ForkReceiptManifest{
			"collection-RESTOREC.1.yaml": {
				SchemaVersion: 1,
				Fork: ForkReceiptRecord{
					EntityType: "collection",
					EntityID:   "RESTOREC.1",
					Source: ForkReceiptSource{
						ProjectID: "LA",
						EntityID:  "LAC.4",
						Version:   "1.0.0",
					},
				},
			},
		},
	}

	plan := BuildRestorePlan(snapshot)
	mat := NewMaterializer(pool, t.TempDir(), nil)
	if err := mat.HydrateRestorePlan(ctx, plan); err != nil {
		t.Fatalf("HydrateRestorePlan: %v", err)
	}

	category, err := queries.WeaveGetCategoryByIdentifier(ctx, sqlcgen.WeaveGetCategoryByIdentifierParams{
		ProjectID:  projectID,
		SemanticID: stringPtr("RESTORE.CAT.1"),
	})
	if err != nil {
		t.Fatalf("get restored category: %v", err)
	}
	if category.CanonicalOrder != 2 {
		t.Fatalf("expected category order 2, got %d", category.CanonicalOrder)
	}

	field, err := queries.WeaveGetFieldByIdentifier(ctx, sqlcgen.WeaveGetFieldByIdentifierParams{
		ProjectID:  projectID,
		SemanticID: stringPtr("RESTOREF.1"),
	})
	if err != nil {
		t.Fatalf("get restored field: %v", err)
	}
	if field.OntologyPath == nil || !strings.Contains(*field.OntologyPath, "crm:P1_is_identified_by") {
		got := "<nil>"
		if field.OntologyPath != nil {
			got = *field.OntologyPath
		}
		t.Fatalf("expected ontology path to include crm:P1_is_identified_by, got %q", got)
	}

	model, err := queries.WeaveGetModelByIdentifier(ctx, sqlcgen.WeaveGetModelByIdentifierParams{
		ProjectID:  projectID,
		Identifier: "RESTOREM.1",
	})
	if err != nil {
		t.Fatalf("get restored model: %v", err)
	}
	if model.ID != "RESTOREM.1" {
		t.Fatalf("expected model id RESTOREM.1, got %q", model.ID)
	}

	if _, err := queries.WeaveGetModelByIdentifier(ctx, sqlcgen.WeaveGetModelByIdentifierParams{
		ProjectID:  projectID,
		Identifier: "LAM.13",
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected adopted vendored model to be skipped, got err=%v", err)
	}

	collection, err := queries.WeaveGetCollectionByIdentifier(ctx, sqlcgen.WeaveGetCollectionByIdentifierParams{
		ProjectID:  projectID,
		Identifier: "RESTOREC.1",
	})
	if err != nil {
		t.Fatalf("get restored collection: %v", err)
	}
	if collection.CanonicalCollectionOrder == nil || *collection.CanonicalCollectionOrder != 4 {
		t.Fatalf("expected collection order 4, got %#v", collection.CanonicalCollectionOrder)
	}
	if !collection.Deprecated {
		t.Fatal("expected restored collection to be deprecated")
	}

	baseOverride, err := queries.WeaveGetBaseOverride(ctx, sqlcgen.WeaveGetBaseOverrideParams{
		FieldID:   field.ID,
		ProjectID: projectID,
	})
	if err != nil {
		t.Fatalf("get base override: %v", err)
	}
	if baseOverride.Position != 1 || baseOverride.IsRequired == nil || !*baseOverride.IsRequired {
		t.Fatalf("unexpected base override: %#v", baseOverride)
	}
	// category_id must be re-resolved against the just-restored category's
	// real (freshly-minted) ULID, not copied verbatim from the snapshot —
	// categories always get a fresh ULID on restore (hydrateCategoryFile),
	// so a literal copy of the snapshot's category_id would reference a
	// category row that no longer exists.
	if baseOverride.CategoryID == nil || *baseOverride.CategoryID != category.ID {
		t.Fatalf("expected base override category_id %q, got %#v", category.ID, baseOverride.CategoryID)
	}

	modelOverrides, err := queries.WeaveListOverridesForEntity(ctx, sqlcgen.WeaveListOverridesForEntityParams{
		EntityType: "model",
		EntityID:   "RESTOREM.1",
	})
	if err != nil {
		t.Fatalf("list model overrides: %v", err)
	}
	if len(modelOverrides) != 1 {
		t.Fatalf("expected 1 model override, got %#v", modelOverrides)
	}
	if modelOverrides[0].CategoryID == nil || *modelOverrides[0].CategoryID != category.ID {
		t.Fatalf("expected model override category_id %q, got %#v", category.ID, modelOverrides[0].CategoryID)
	}
	modelRefs, err := queries.WeaveListOverrideRefs(ctx, modelOverrides[0].ID)
	if err != nil {
		t.Fatalf("list model override refs: %v", err)
	}
	if len(modelRefs) != 2 || modelRefs[0].TargetID != "RESTOREC.1" && modelRefs[1].TargetID != "RESTOREC.1" {
		t.Fatalf("unexpected model refs: %#v", modelRefs)
	}

	collectionOverrides, err := queries.WeaveListOverridesForEntity(ctx, sqlcgen.WeaveListOverridesForEntityParams{
		EntityType: "collection",
		EntityID:   "RESTOREC.1",
	})
	if err != nil {
		t.Fatalf("list collection overrides: %v", err)
	}
	if len(collectionOverrides) != 1 || collectionOverrides[0].SetValue == nil || *collectionOverrides[0].SetValue != "fixed" {
		t.Fatalf("unexpected collection override: %#v", collectionOverrides)
	}

	adoptions, err := mat.loadProjectAdoptions(ctx, projectID)
	if err != nil {
		t.Fatalf("load project adoptions: %v", err)
	}
	if len(adoptions) != 1 || adoptions[0].SourceEntityID != "LAM.13" {
		t.Fatalf("unexpected adoptions: %#v", adoptions)
	}
	forks, err := mat.loadProjectForks(ctx, projectID)
	if err != nil {
		t.Fatalf("load project forks: %v", err)
	}
	if len(forks) != 1 || forks[0].SourceEntityID != "LAC.4" {
		t.Fatalf("unexpected forks: %#v", forks)
	}
}
