package gitmaterializer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
)

func TestLoadProjectSnapshot(t *testing.T) {
	snapshot := loadSnapshotFixture(t)

	if snapshot.Manifest.Project.ID != "TPC" {
		t.Fatalf("expected project id TPC, got %q", snapshot.Manifest.Project.ID)
	}
	if snapshot.Mod == nil || snapshot.Mod.Module.ProjectID != "TPC" {
		t.Fatalf("expected pletka.mod project TPC, got %#v", snapshot.Mod)
	}
	if len(snapshot.Sum) != 2 {
		t.Fatalf("expected 2 pletka.sum entries, got %d", len(snapshot.Sum))
	}
	if snapshot.Adoptions == nil || len(snapshot.Adoptions.Receipts) != 1 {
		t.Fatalf("expected 1 adoption receipt, got %#v", snapshot.Adoptions)
	}
	if snapshot.Forks == nil || len(snapshot.Forks.Receipts) != 1 {
		t.Fatalf("expected 1 fork receipt, got %#v", snapshot.Forks)
	}
	if len(snapshot.Entities.Categories) != 1 ||
		len(snapshot.Entities.Fields) != 1 ||
		len(snapshot.Entities.Models) != 1 ||
		len(snapshot.Entities.Collections) != 1 ||
		len(snapshot.Entities.BaseOverrides) != 1 ||
		len(snapshot.Entities.ModelOverrides) != 1 ||
		len(snapshot.Entities.CollectionOverrides) != 1 {
		t.Fatalf("unexpected entity inventory counts: %#v", snapshot.Entities)
	}
	if len(snapshot.Vendor.Projects) != 1 {
		t.Fatalf("expected 1 vendored project snapshot, got %d", len(snapshot.Vendor.Projects))
	}
	if snapshot.Vendor.Projects[0].Module != "pletka.io/orgs/test-org/projects/LA" {
		t.Fatalf("unexpected vendored project module: %q", snapshot.Vendor.Projects[0].Module)
	}
	if snapshot.Vendor.Projects[0].Snapshot.Manifest.Project.ID != "LA" {
		t.Fatalf("expected vendored project LA, got %q", snapshot.Vendor.Projects[0].Snapshot.Manifest.Project.ID)
	}
	if snapshot.Manifest.Inheritance == nil || len(snapshot.Manifest.Inheritance.Parents) != 1 {
		t.Fatalf("expected one parent selector, got %#v", snapshot.Manifest.Inheritance)
	}
	if got := snapshot.Manifest.Inheritance.Parents[0]; got.SourceMode != "release" || got.SourceVersion != "1.2.0" {
		t.Fatalf("expected pinned parent selector, got %#v", got)
	}
	if len(snapshot.Vendor.Ontologies) != 1 {
		t.Fatalf("expected 1 vendored ontology snapshot, got %d", len(snapshot.Vendor.Ontologies))
	}
	if snapshot.Vendor.Ontologies[0].Module != "ontology.pletka.io/linked-art" {
		t.Fatalf("unexpected vendored ontology module: %q", snapshot.Vendor.Ontologies[0].Module)
	}
	if len(snapshot.Vendor.Ontologies[0].Snapshot.Sources) != 1 {
		t.Fatalf("expected vendored ontology sources, got %#v", snapshot.Vendor.Ontologies[0].Snapshot.Sources)
	}
}

func loadSnapshotFixture(t *testing.T) *ProjectSnapshot {
	t.Helper()
	root := t.TempDir()

	projectManifestPayload, err := encodeProjectManifest(projectManifest{
		SchemaVersion: 1,
		Project: projectManifestProject{
			ID:         "TPC",
			Namespace:  "https://example.org/ns/",
			Visibility: "private",
			Owner: &manifestActor{
				ActorID:     "org-1",
				Type:        "organization",
				Slug:        "test-org",
				DisplayName: "Test Org",
			},
		},
		Inheritance: &projectManifestInheritance{
			Parents: []projectManifestParent{
				{ProjectID: "LA", IsPrimary: true, CanonicalOrder: 0, SourceMode: "release", SourceVersion: "1.2.0"},
			},
		},
		Manifests: &projectManifestManifests{
			Adoptions: "adoptions/index.yaml",
			Forks:     "forks/index.yaml",
		},
	})
	if err != nil {
		t.Fatalf("encode project manifest: %v", err)
	}
	if err := writeEntityFile(root, domain.FilePath(domain.PathSpec{EntityType: "project"}), projectManifestPayload); err != nil {
		t.Fatalf("write project manifest: %v", err)
	}

	modPayload, err := encodePletkaModManifest(pletkaModManifest{
		SchemaVersion: 1,
		Module: pletkaModModule{
			Path:      "pletka.io/orgs/test-org/projects/TPC",
			ProjectID: "TPC",
		},
		Require: pletkaModRequire{
			Projects: []pletkaProjectRequirement{
				{
					Module:    "pletka.io/orgs/test-org/projects/LA",
					ProjectID: "LA",
					Mode:      "parent",
					Version:   "1.2.0",
				},
			},
			Ontologies: []pletkaOntologyRequirement{
				{
					Module:            "ontology.pletka.io/linked-art",
					OntologyID:        "linked-art",
					OntologyVersionID: "la-1",
					Version:           "1.1.0",
				},
			},
		},
		Replace: []map[string]any{},
	})
	if err != nil {
		t.Fatalf("encode pletka.mod: %v", err)
	}
	if err := writeEntityFile(root, "pletka.mod", modPayload); err != nil {
		t.Fatalf("write pletka.mod: %v", err)
	}
	if err := writePletkaSum(root, []pletkaSumEntry{
		{
			Module:  "pletka.io/orgs/test-org/projects/LA",
			Version: "1.2.0",
			TreeSHA: "abc123",
		},
		{
			Module:  "ontology.pletka.io/linked-art",
			Version: "1.1.0",
			TreeSHA: "def456",
		},
	}); err != nil {
		t.Fatalf("write pletka.sum: %v", err)
	}

	adoptionsIndexPayload, err := encodeAdoptionsIndexManifest(adoptionsIndexManifest{
		SchemaVersion: 1,
		Receipts: []adoptionsIndexReceiptEntry{
			{
				EntityType: "model",
				EntityID:   "LAM.13",
				File:       "model-LAM.13.yaml",
			},
		},
	})
	if err != nil {
		t.Fatalf("encode adoptions index: %v", err)
	}
	if err := writeEntityFile(root, "adoptions/index.yaml", adoptionsIndexPayload); err != nil {
		t.Fatalf("write adoptions index: %v", err)
	}
	adoptionReceiptPayload, err := encodeAdoptionReceiptManifest(adoptionReceiptManifest{
		SchemaVersion: 1,
		Adoption: adoptionReceiptRecord{
			EntityType: "model",
			EntityID:   "LAM.13",
			Source: adoptionReceiptSource{
				ProjectID: "LA",
				EntityID:  "LAM.13",
				Version:   "1.0.0",
			},
			Contexts: []adoptionReceiptContext{
				{
					ContextEntityType: "project",
					ContextEntityID:   "TPC",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("encode adoption receipt: %v", err)
	}
	if err := writeEntityFile(root, "adoptions/model-LAM.13.yaml", adoptionReceiptPayload); err != nil {
		t.Fatalf("write adoption receipt: %v", err)
	}

	forksIndexPayload, err := encodeForksIndexManifest(forksIndexManifest{
		SchemaVersion: 1,
		Receipts: []forksIndexReceiptEntry{
			{
				EntityType: "collection",
				EntityID:   "TPCC.4",
				File:       "collection-TPCC.4.yaml",
			},
		},
	})
	if err != nil {
		t.Fatalf("encode forks index: %v", err)
	}
	if err := writeEntityFile(root, "forks/index.yaml", forksIndexPayload); err != nil {
		t.Fatalf("write forks index: %v", err)
	}
	forkReceiptPayload, err := encodeForkReceiptManifest(forkReceiptManifest{
		SchemaVersion: 1,
		Fork: forkReceiptRecord{
			EntityType: "collection",
			EntityID:   "TPCC.4",
			Source: forkReceiptSource{
				ProjectID: "LA",
				EntityID:  "LAC.4",
				Version:   "1.0.0",
			},
		},
	})
	if err != nil {
		t.Fatalf("encode fork receipt: %v", err)
	}
	if err := writeEntityFile(root, "forks/collection-TPCC.4.yaml", forkReceiptPayload); err != nil {
		t.Fatalf("write fork receipt: %v", err)
	}

	writeCanonical := func(rel string, payload map[string]any) {
		t.Helper()
		raw, err := canonical.Encode(payload)
		if err != nil {
			t.Fatalf("encode %s: %v", rel, err)
		}
		if err := writeEntityFile(root, rel, raw); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	writeCanonical("categories/TPC.CAT.1.yaml", map[string]any{"id": "TPC.CAT.1"})
	writeCanonical("fields/TPCF.1/field.yaml", map[string]any{"id": "TPCF.1"})
	writeCanonical("fields/TPCF.1/base-override.yaml", map[string]any{"id": 1})
	writeCanonical("models/TPCM.1/model.yaml", map[string]any{"id": "TPCM.1"})
	writeCanonical("models/TPCM.1/overrides/TPCF.1@77.yaml", map[string]any{"id": 77})
	writeCanonical("collections/TPCC.1/collection.yaml", map[string]any{"id": "TPCC.1"})
	writeCanonical("collections/TPCC.1/overrides/TPCF.1@88.yaml", map[string]any{"id": 88})

	vendorProjectRoot := filepath.Join(root, "vendor", "projects", "pletka.io", "orgs", "test-org", "projects", "LA")
	if err := os.MkdirAll(vendorProjectRoot, 0o755); err != nil {
		t.Fatalf("mkdir vendored project: %v", err)
	}
	childProjectManifestPayload, err := encodeProjectManifest(projectManifest{
		SchemaVersion: 1,
		Project:       projectManifestProject{ID: "LA"},
		Manifests: &projectManifestManifests{
			Adoptions: "adoptions/index.yaml",
			Forks:     "forks/index.yaml",
		},
	})
	if err != nil {
		t.Fatalf("encode vendored project manifest: %v", err)
	}
	if err := writeEntityFile(vendorProjectRoot, "project.yaml", childProjectManifestPayload); err != nil {
		t.Fatalf("write vendored project manifest: %v", err)
	}
	childModPayload, err := encodePletkaModManifest(pletkaModManifest{
		SchemaVersion: 1,
		Module: pletkaModModule{
			Path:      "pletka.io/orgs/test-org/projects/LA",
			ProjectID: "LA",
		},
		Require: pletkaModRequire{
			Projects:   []pletkaProjectRequirement{},
			Ontologies: []pletkaOntologyRequirement{},
		},
		Replace: []map[string]any{},
	})
	if err != nil {
		t.Fatalf("encode vendored project mod: %v", err)
	}
	if err := writeEntityFile(vendorProjectRoot, "pletka.mod", childModPayload); err != nil {
		t.Fatalf("write vendored project mod: %v", err)
	}
	emptyAdoptionsPayload, err := encodeAdoptionsIndexManifest(adoptionsIndexManifest{
		SchemaVersion: 1,
		Receipts:      []adoptionsIndexReceiptEntry{},
	})
	if err != nil {
		t.Fatalf("encode empty adoptions index: %v", err)
	}
	if err := writeEntityFile(vendorProjectRoot, "adoptions/index.yaml", emptyAdoptionsPayload); err != nil {
		t.Fatalf("write vendored adoptions index: %v", err)
	}
	emptyForksPayload, err := encodeForksIndexManifest(forksIndexManifest{
		SchemaVersion: 1,
		Receipts:      []forksIndexReceiptEntry{},
	})
	if err != nil {
		t.Fatalf("encode empty forks index: %v", err)
	}
	if err := writeEntityFile(vendorProjectRoot, "forks/index.yaml", emptyForksPayload); err != nil {
		t.Fatalf("write vendored forks index: %v", err)
	}

	vendorOntologyRoot := filepath.Join(root, "vendor", "ontologies", "ontology.pletka.io", "linked-art", "1.1.0")
	ontologyPayload, err := encodeOntologyVendorManifest(ontologyVendorManifest{
		SchemaVersion: 1,
		Ontology: ontologyVendorManifestRoot{
			Module:     "ontology.pletka.io/linked-art",
			OntologyID: "linked-art",
			VersionID:  "la-1",
			Version:    "1.1.0",
			Slug:       "linked-art",
		},
		Sources: &ontologyVendorSources{
			Files: []string{"src/linked-art.ttl"},
		},
	})
	if err != nil {
		t.Fatalf("encode ontology manifest: %v", err)
	}
	if err := writeEntityFile(vendorOntologyRoot, "ontology.yaml", ontologyPayload); err != nil {
		t.Fatalf("write ontology manifest: %v", err)
	}
	if err := writeEntityFile(vendorOntologyRoot, "src/linked-art.ttl", []byte("@prefix la: <https://linked.art/> .\n")); err != nil {
		t.Fatalf("write ontology source: %v", err)
	}

	snapshot, err := LoadProjectSnapshot(root)
	if err != nil {
		t.Fatalf("LoadProjectSnapshot: %v", err)
	}
	return snapshot
}

func TestLoadProjectSnapshot_MissingReferencedAdoptionReceipt(t *testing.T) {
	root := t.TempDir()

	projectManifestPayload, err := encodeProjectManifest(projectManifest{
		SchemaVersion: 1,
		Project:       projectManifestProject{ID: "TPC"},
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
			Path:      "pletka.io/orgs/test-org/projects/TPC",
			ProjectID: "TPC",
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
		Receipts: []adoptionsIndexReceiptEntry{
			{
				EntityType: "model",
				EntityID:   "LAM.13",
				File:       "model-LAM.13.yaml",
			},
		},
	})
	if err != nil {
		t.Fatalf("encode adoptions index: %v", err)
	}
	if err := writeEntityFile(root, "adoptions/index.yaml", adoptionsIndexPayload); err != nil {
		t.Fatalf("write adoptions index: %v", err)
	}
	emptyForksPayload, err := encodeForksIndexManifest(forksIndexManifest{
		SchemaVersion: 1,
		Receipts:      []forksIndexReceiptEntry{},
	})
	if err != nil {
		t.Fatalf("encode forks index: %v", err)
	}
	if err := writeEntityFile(root, "forks/index.yaml", emptyForksPayload); err != nil {
		t.Fatalf("write forks index: %v", err)
	}

	_, err = LoadProjectSnapshot(root)
	if err == nil {
		t.Fatal("expected missing adoption receipt error")
	}
	if got := err.Error(); got == "" || !containsAll(got, "load adoption receipt", "model-LAM.13.yaml") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadProjectSnapshot_MismatchedParentDependencyVersion(t *testing.T) {
	root := t.TempDir()

	projectManifestPayload, err := encodeProjectManifest(projectManifest{
		SchemaVersion: 1,
		Project:       projectManifestProject{ID: "TPC"},
		Inheritance: &projectManifestInheritance{
			Parents: []projectManifestParent{
				{ProjectID: "LA", IsPrimary: true, CanonicalOrder: 0, SourceMode: "release", SourceVersion: "1.2.0"},
			},
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
			Path:      "pletka.io/orgs/test-org/projects/TPC",
			ProjectID: "TPC",
		},
		Require: pletkaModRequire{
			Projects: []pletkaProjectRequirement{
				{Module: "pletka.io/orgs/test-org/projects/LA", ProjectID: "LA", Mode: "parent", Version: "draft"},
			},
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

	_, err = LoadProjectSnapshot(root)
	if err == nil || !strings.Contains(err.Error(), "version mismatch") {
		t.Fatalf("expected version mismatch error, got %v", err)
	}
}

func TestLoadProjectSnapshot_VersionedVendoredProjectDir(t *testing.T) {
	root := t.TempDir()

	projectManifestPayload, err := encodeProjectManifest(projectManifest{
		SchemaVersion: 1,
		Project:       projectManifestProject{ID: "TPC"},
		Inheritance: &projectManifestInheritance{
			Parents: []projectManifestParent{
				{ProjectID: "LA", IsPrimary: true, CanonicalOrder: 0, SourceMode: "release", SourceVersion: "1.2.0"},
			},
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
			Path:      "pletka.io/orgs/test-org/projects/TPC",
			ProjectID: "TPC",
		},
		Require: pletkaModRequire{
			Projects: []pletkaProjectRequirement{
				{Module: "pletka.io/orgs/test-org/projects/LA", ProjectID: "LA", Mode: "parent", Version: "1.2.0"},
			},
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
	if err := writePletkaSum(root, []pletkaSumEntry{
		{Module: "pletka.io/orgs/test-org/projects/LA", Version: "1.2.0", TreeSHA: "abc123"},
	}); err != nil {
		t.Fatalf("write pletka.sum: %v", err)
	}
	emptyAdoptionsPayload, err := encodeAdoptionsIndexManifest(adoptionsIndexManifest{SchemaVersion: 1, Receipts: []adoptionsIndexReceiptEntry{}})
	if err != nil {
		t.Fatalf("encode empty adoptions index: %v", err)
	}
	if err := writeEntityFile(root, "adoptions/index.yaml", emptyAdoptionsPayload); err != nil {
		t.Fatalf("write adoptions index: %v", err)
	}
	emptyForksPayload, err := encodeForksIndexManifest(forksIndexManifest{SchemaVersion: 1, Receipts: []forksIndexReceiptEntry{}})
	if err != nil {
		t.Fatalf("encode empty forks index: %v", err)
	}
	if err := writeEntityFile(root, "forks/index.yaml", emptyForksPayload); err != nil {
		t.Fatalf("write forks index: %v", err)
	}

	vendorProjectRoot := filepath.Join(root, "vendor", "projects", "pletka.io", "orgs", "test-org", "projects", "LA", "1.2.0")
	childProjectManifestPayload, err := encodeProjectManifest(projectManifest{
		SchemaVersion: 1,
		Project:       projectManifestProject{ID: "LA"},
		Manifests: &projectManifestManifests{
			Adoptions: "adoptions/index.yaml",
			Forks:     "forks/index.yaml",
		},
	})
	if err != nil {
		t.Fatalf("encode vendored project manifest: %v", err)
	}
	if err := writeEntityFile(vendorProjectRoot, "project.yaml", childProjectManifestPayload); err != nil {
		t.Fatalf("write vendored project manifest: %v", err)
	}
	childModPayload, err := encodePletkaModManifest(pletkaModManifest{
		SchemaVersion: 1,
		Module: pletkaModModule{
			Path:      "pletka.io/orgs/test-org/projects/LA",
			ProjectID: "LA",
		},
		Require: pletkaModRequire{
			Projects:   []pletkaProjectRequirement{},
			Ontologies: []pletkaOntologyRequirement{},
		},
		Replace: []map[string]any{},
	})
	if err != nil {
		t.Fatalf("encode vendored project mod: %v", err)
	}
	if err := writeEntityFile(vendorProjectRoot, "pletka.mod", childModPayload); err != nil {
		t.Fatalf("write vendored project mod: %v", err)
	}
	if err := writeEntityFile(vendorProjectRoot, "adoptions/index.yaml", emptyAdoptionsPayload); err != nil {
		t.Fatalf("write vendored adoptions index: %v", err)
	}
	if err := writeEntityFile(vendorProjectRoot, "forks/index.yaml", emptyForksPayload); err != nil {
		t.Fatalf("write vendored forks index: %v", err)
	}

	snapshot, err := LoadProjectSnapshot(root)
	if err != nil {
		t.Fatalf("LoadProjectSnapshot: %v", err)
	}
	if len(snapshot.Vendor.Projects) != 1 {
		t.Fatalf("expected 1 vendored project, got %d", len(snapshot.Vendor.Projects))
	}
	if snapshot.Vendor.Projects[0].Module != "pletka.io/orgs/test-org/projects/LA" {
		t.Fatalf("unexpected vendored project module: %q", snapshot.Vendor.Projects[0].Module)
	}
	if snapshot.Vendor.Projects[0].Snapshot.Manifest.Project.ID != "LA" {
		t.Fatalf("unexpected vendored project payload: %#v", snapshot.Vendor.Projects[0].Snapshot.Manifest.Project)
	}
}

func TestBuildRestorePlan(t *testing.T) {
	snapshot := loadSnapshotFixture(t)
	plan := BuildRestorePlan(snapshot)

	if plan.ProjectID != "TPC" {
		t.Fatalf("expected project id TPC, got %q", plan.ProjectID)
	}
	if plan.ModulePath != "pletka.io/orgs/test-org/projects/TPC" {
		t.Fatalf("unexpected module path: %q", plan.ModulePath)
	}
	if len(plan.Operations) != 13 {
		t.Fatalf("expected 13 restore operations, got %d", len(plan.Operations))
	}

	wantKinds := []RestoreOperationKind{
		RestoreVendoredProject,
		RestoreProjectManifest,
		RestoreVendoredOntology,
		RestoreProjectManifest,
		RestoreCategory,
		RestoreField,
		RestoreModel,
		RestoreCollection,
		RestoreBaseOverride,
		RestoreModelOverride,
		RestoreCollectionOverride,
		RestoreAdoptionReceipt,
		RestoreForkReceipt,
	}
	for i, want := range wantKinds {
		if plan.Operations[i].Kind != want {
			t.Fatalf("operation %d: expected %s, got %s", i, want, plan.Operations[i].Kind)
		}
	}
	if plan.Operations[0].Module != "pletka.io/orgs/test-org/projects/LA" {
		t.Fatalf("unexpected vendored project module: %q", plan.Operations[0].Module)
	}
	if plan.Operations[2].Module != "ontology.pletka.io/linked-art" {
		t.Fatalf("unexpected vendored ontology module: %q", plan.Operations[2].Module)
	}
	if plan.Operations[5].Path != "fields/TPCF.1/field.yaml" {
		t.Fatalf("unexpected field path: %q", plan.Operations[5].Path)
	}
	if plan.Operations[11].Path != "adoptions/model-LAM.13.yaml" {
		t.Fatalf("unexpected adoption receipt path: %q", plan.Operations[11].Path)
	}
	if plan.Operations[12].Path != "forks/collection-TPCC.4.yaml" {
		t.Fatalf("unexpected fork receipt path: %q", plan.Operations[12].Path)
	}
}

func TestBuildRestorePreview(t *testing.T) {
	snapshot := loadSnapshotFixture(t)
	plan := BuildRestorePlan(snapshot)
	preview := BuildRestorePreview(snapshot, plan)

	if preview.ProjectID != "TPC" {
		t.Fatalf("expected project id TPC, got %q", preview.ProjectID)
	}
	if preview.ModulePath != "pletka.io/orgs/test-org/projects/TPC" {
		t.Fatalf("unexpected module path: %q", preview.ModulePath)
	}
	if !preview.HasLockfile {
		t.Fatalf("expected lockfile to be detected")
	}
	if !preview.SelfContained {
		t.Fatalf("expected preview to be self-contained")
	}
	if preview.Counts.Categories != 1 ||
		preview.Counts.Fields != 1 ||
		preview.Counts.Models != 1 ||
		preview.Counts.Collections != 1 {
		t.Fatalf("unexpected entity counts: %#v", preview.Counts)
	}
	if preview.Counts.AdoptionReceipts != 1 || preview.Counts.ForkReceipts != 1 {
		t.Fatalf("unexpected receipt counts: %#v", preview.Counts)
	}
	if preview.Counts.VendoredProjects != 1 || preview.Counts.VendoredOntologies != 1 {
		t.Fatalf("unexpected vendor counts: %#v", preview.Counts)
	}
	if preview.Counts.Operations != len(plan.Operations) {
		t.Fatalf("expected %d operations, got %d", len(plan.Operations), preview.Counts.Operations)
	}
	if len(preview.ParentDependencies) != 1 {
		t.Fatalf("expected one parent dependency, got %#v", preview.ParentDependencies)
	}
	if got := preview.ParentDependencies[0]; got.ProjectID != "LA" || got.SourceMode != "release" || got.SourceVersion != "1.2.0" {
		t.Fatalf("unexpected parent dependency: %#v", got)
	}
	if len(preview.OntologyDependencies) != 1 {
		t.Fatalf("expected one ontology dependency, got %#v", preview.OntologyDependencies)
	}
	if got := preview.OntologyDependencies[0]; got.OntologyID != "linked-art" || got.Version != "1.1.0" {
		t.Fatalf("unexpected ontology dependency: %#v", got)
	}
	if len(preview.OperationCounts) == 0 {
		t.Fatalf("expected operation counts")
	}
	var foundModel bool
	for _, stat := range preview.OperationCounts {
		if stat.Kind == RestoreModel && stat.Count == 1 {
			foundModel = true
			break
		}
	}
	if !foundModel {
		t.Fatalf("expected RestoreModel count 1 in %#v", preview.OperationCounts)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestLoadOntologySnapshot_RejectsUnsafeSourcePaths(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{name: "path traversal", file: "../escape.ttl"},
		{name: "nested path traversal", file: "src/../../escape.ttl"},
		{name: "absolute path", file: "/etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			payload, err := encodeOntologyVendorManifest(ontologyVendorManifest{
				SchemaVersion: 1,
				Ontology: ontologyVendorManifestRoot{
					Module:     "ontology.pletka.io/unsafe",
					OntologyID: "unsafe",
					VersionID:  "unsafe-1",
					Version:    "1.0.0",
					Slug:       "unsafe",
				},
				Sources: &ontologyVendorSources{
					Files: []string{tt.file},
				},
			})
			if err != nil {
				t.Fatalf("encode ontology manifest: %v", err)
			}
			if err := writeEntityFile(root, "ontology.yaml", payload); err != nil {
				t.Fatalf("write ontology manifest: %v", err)
			}

			_, err = loadOntologySnapshot(root)
			if err == nil {
				t.Fatal("loadOntologySnapshot() = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.file) {
				t.Fatalf("error %q does not name the unsafe file %q", err.Error(), tt.file)
			}
		})
	}
}
