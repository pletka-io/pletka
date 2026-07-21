//go:build integration

package gitmaterializer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// fakeImporter is a test double for VendoredOntologyImporter that records
// the ontology version id of every call it receives.
type fakeImporter struct {
	calls []string
	err   error
}

func (f *fakeImporter) ImportVendoredOntology(ctx context.Context, imp VendoredOntologyImport) error {
	f.calls = append(f.calls, imp.Manifest.Ontology.VersionID)
	return f.err
}

// buildOntologyImportFixture creates a temp vendor tree with one vendored
// ontology whose manifest carries versionID, hashes it with
// hashDirectoryTree, and returns a ProjectSnapshot whose Sum entry matches
// that hash. Callers can tamper with the returned directory to exercise the
// checksum-mismatch path.
func buildOntologyImportFixture(t *testing.T, versionID string) (snapshot *ProjectSnapshot, ontologyDir string) {
	t.Helper()
	root := t.TempDir()

	ontologyDir = filepath.Join(root, "vendor", "ontologies", "example.org", "ontology", "core", "1.0.0")
	if err := os.MkdirAll(filepath.Join(ontologyDir, "src"), 0o755); err != nil {
		t.Fatalf("mkdir ontology dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ontologyDir, "src", "core.ttl"), []byte("@prefix ex: <http://example.org/> .\n"), 0o644); err != nil {
		t.Fatalf("write ontology source: %v", err)
	}

	manifest := OntologyVendorManifest{
		SchemaVersion: 1,
		Ontology: OntologyVendorManifestRoot{
			Module:     "example.org/ontology/core",
			OntologyID: "core-ontology",
			VersionID:  versionID,
			Version:    "1.0.0",
			Slug:       "core",
		},
		Sources: &OntologyVendorSources{Files: []string{"src/core.ttl"}},
	}
	payload, err := encodeOntologyVendorManifest(manifest)
	if err != nil {
		t.Fatalf("encode ontology manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ontologyDir, "ontology.yaml"), payload, 0o644); err != nil {
		t.Fatalf("write ontology.yaml: %v", err)
	}

	hash, err := hashDirectoryTree(ontologyDir)
	if err != nil {
		t.Fatalf("hash ontology dir: %v", err)
	}

	snapshot = &ProjectSnapshot{
		RootDir: root,
		Sum: []PletkaSumEntry{
			{Module: "example.org/ontology/core", Version: "1.0.0", TreeSHA: hash},
		},
		Vendor: VendorSnapshotSet{
			Ontologies: []VendoredOntologySnapshot{
				{
					Module:  "example.org/ontology/core",
					Version: "1.0.0",
					Snapshot: &OntologySnapshot{
						RootDir:  ontologyDir,
						Manifest: manifest,
					},
				},
			},
		},
	}
	return snapshot, ontologyDir
}

func TestHydrateVendoredOntologies(t *testing.T) {
	ctx := context.Background()

	t.Run("importer called once per vendored ontology not in database", func(t *testing.T) {
		pool := hydrateTestPool(t)
		snapshot, _ := buildOntologyImportFixture(t, "hydrate-ontology-not-present")
		plan := &RestorePlan{Snapshot: snapshot}

		imp := &fakeImporter{}
		mat := NewMaterializer(pool, t.TempDir(), nil).WithOntologyImporter(imp)

		if err := mat.hydrateVendoredOntologies(ctx, plan); err != nil {
			t.Fatalf("hydrateVendoredOntologies() = %v, want nil", err)
		}
		if len(imp.calls) != 1 || imp.calls[0] != "hydrate-ontology-not-present" {
			t.Fatalf("expected importer called once with version id, got %#v", imp.calls)
		}
	})

	t.Run("version already in database skips the importer", func(t *testing.T) {
		pool := hydrateTestPool(t)
		queries := sqlcgen.New(pool)

		ontologyID := "hydrate-existing-ontology"
		ontologyVersionID := "hydrate-existing-ontology-v1"
		creatorID := "HYDRATE_ONTOLOGY_CREATOR"

		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE id = $1`, ontologyVersionID)
			_, _ = pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, ontologyID)
			_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, creatorID)
		})

		if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
			ID:          creatorID,
			Type:        "person",
			DisplayName: "Hydrate Ontology Creator",
			Slug:        "hydrate-ontology-creator",
			Role:        "",
			Visibility:  "private",
		}); err != nil {
			t.Fatalf("create creator actor: %v", err)
		}
		if _, err := queries.WeaveCreateOntology(ctx, sqlcgen.WeaveCreateOntologyParams{
			ID:           ontologyID,
			Prefix:       "ho",
			Namespace:    "https://example.org/hydrate-ontology/",
			Name:         "Hydrate Existing Ontology",
			Description:  []byte(`{"en":"Existing ontology"}`),
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
			RdfContent:             stringPtr("@prefix ho: <https://example.org/hydrate-ontology/> .\n"),
			ParsedAt:               pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			VersionInfo:            []byte(`{"en":"1.0.0"}`),
			ImportedOntologies:     []string{},
			OntologyLabel:          []byte(`{"en":"Hydrate Existing Ontology"}`),
			OntologyComment:        []byte(`{"en":"Comment"}`),
			OntologyMetadata:       []byte(`{}`),
			ClassCount:             1,
			PropertyCount:          1,
		}); err != nil {
			t.Fatalf("create ontology version: %v", err)
		}

		snapshot, _ := buildOntologyImportFixture(t, ontologyVersionID)
		plan := &RestorePlan{Snapshot: snapshot}

		imp := &fakeImporter{}
		mat := NewMaterializer(pool, t.TempDir(), nil).WithOntologyImporter(imp)

		if err := mat.hydrateVendoredOntologies(ctx, plan); err != nil {
			t.Fatalf("hydrateVendoredOntologies() = %v, want nil", err)
		}
		if len(imp.calls) != 0 {
			t.Fatalf("expected importer not called, got %#v", imp.calls)
		}
	})

	t.Run("vendored ontologies with no importer wired errors", func(t *testing.T) {
		snapshot, _ := buildOntologyImportFixture(t, "hydrate-ontology-no-importer")
		plan := &RestorePlan{Snapshot: snapshot}

		mat := NewMaterializer(nil, t.TempDir(), nil)

		err := mat.hydrateVendoredOntologies(ctx, plan)
		if err == nil {
			t.Fatal("hydrateVendoredOntologies() = nil, want error")
		}
		if !strings.Contains(err.Error(), "no ontology importer is wired") {
			t.Fatalf("error %q does not describe the missing importer", err.Error())
		}
	})

	t.Run("checksum mismatch aborts before the importer is called", func(t *testing.T) {
		snapshot, ontologyDir := buildOntologyImportFixture(t, "hydrate-ontology-tampered")
		if err := os.WriteFile(filepath.Join(ontologyDir, "src", "core.ttl"), []byte("tampered"), 0o644); err != nil {
			t.Fatalf("tamper ontology source: %v", err)
		}
		plan := &RestorePlan{Snapshot: snapshot}

		imp := &fakeImporter{}
		mat := NewMaterializer(nil, t.TempDir(), nil).WithOntologyImporter(imp)

		err := mat.hydrateVendoredOntologies(ctx, plan)
		if err == nil {
			t.Fatal("hydrateVendoredOntologies() = nil, want error")
		}
		if !strings.Contains(err.Error(), "checksum mismatch") {
			t.Fatalf("error %q does not describe a checksum mismatch", err.Error())
		}
		if len(imp.calls) != 0 {
			t.Fatalf("expected importer not called, got %#v", imp.calls)
		}
	})
}

func TestHydrateRestorePlan(t *testing.T) {
	ctx := context.Background()

	t.Run("HydrateRestorePlan calls hydrateVendoredOntologies before other hydrators", func(t *testing.T) {
		pool := hydrateTestPool(t)
		snapshot, _ := buildOntologyImportFixture(t, "restore-plan-test-ontology")
		plan := &RestorePlan{Snapshot: snapshot}

		// Wire a fakeImporter that will record if/when it's called.
		imp := &fakeImporter{}
		mat := NewMaterializer(pool, t.TempDir(), nil).WithOntologyImporter(imp)

		// Call HydrateRestorePlan. The snapshot is minimal (no project data),
		// so HydrateProjectShell will likely fail. But the assertion is that
		// hydrateVendoredOntologies ran FIRST and called the importer BEFORE
		// that error occurred.
		_ = mat.HydrateRestorePlan(ctx, plan)

		// Verify the importer was called with the vendored ontology version ID.
		// This proves the ordering: hydrateVendoredOntologies ran before returning
		// an error from HydrateProjectShell.
		if len(imp.calls) != 1 || imp.calls[0] != "restore-plan-test-ontology" {
			t.Fatalf("expected importer called once with version id, got %#v", imp.calls)
		}
	})

	t.Run("hydrateVendoredOntologies errors without an importer even if version is in DB", func(t *testing.T) {
		pool := hydrateTestPool(t)
		queries := sqlcgen.New(pool)

		ontologyID := "hydrate-strict-nil-ontology"
		ontologyVersionID := "hydrate-strict-nil-ontology-v1"
		creatorID := "HYDRATE_STRICT_NIL_CREATOR"

		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE id = $1`, ontologyVersionID)
			_, _ = pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, ontologyID)
			_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, creatorID)
		})

		// Insert the ontology and version into the database.
		if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
			ID:          creatorID,
			Type:        "person",
			DisplayName: "Strict Nil Creator",
			Slug:        "strict-nil-creator",
			Role:        "",
			Visibility:  "private",
		}); err != nil {
			t.Fatalf("create creator actor: %v", err)
		}
		if _, err := queries.WeaveCreateOntology(ctx, sqlcgen.WeaveCreateOntologyParams{
			ID:           ontologyID,
			Prefix:       "sn",
			Namespace:    "https://example.org/strict-nil/",
			Name:         "Strict Nil Ontology",
			Description:  []byte(`{"en":"Strict nil ontology"}`),
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
			RdfContent:             stringPtr("@prefix sn: <https://example.org/strict-nil/> .\n"),
			ParsedAt:               pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			VersionInfo:            []byte(`{"en":"1.0.0"}`),
			ImportedOntologies:     []string{},
			OntologyLabel:          []byte(`{"en":"Strict Nil Ontology"}`),
			OntologyComment:        []byte(`{"en":"Comment"}`),
			OntologyMetadata:       []byte(`{}`),
			ClassCount:             1,
			PropertyCount:          1,
		}); err != nil {
			t.Fatalf("create ontology version: %v", err)
		}

		// Build a snapshot that vendors this existing ontology.
		snapshot, _ := buildOntologyImportFixture(t, ontologyVersionID)
		plan := &RestorePlan{Snapshot: snapshot}

		// Create a materializer WITHOUT wiring an importer.
		mat := NewMaterializer(pool, t.TempDir(), nil)

		// Call hydrateVendoredOntologies. Even though the version is in DB,
		// it must error because no importer is wired.
		err := mat.hydrateVendoredOntologies(ctx, plan)
		if err == nil {
			t.Fatal("hydrateVendoredOntologies() = nil, want error")
		}
		if !strings.Contains(err.Error(), "no ontology importer is wired") {
			t.Fatalf("error %q does not contain 'no ontology importer is wired'", err.Error())
		}
	})
}
