//go:build integration

package gitmaterializer_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// moduleSlugForOntology mirrors gitmaterializer's unexported
// ontologyModuleSlug (pletka_mod.go). Duplicated here because this file is in
// the external gitmaterializer_test package: it needs the same slug the
// vendor path derives, to locate the vendored directory of whichever
// ontology currently carries the "defaults" prefix, without asserting
// against a specific ontology name/title.
func moduleSlugForOntology(prefix, name, fallback string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name != "" {
		replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ".", "-", ":", "-")
		name = replacer.Replace(name)
		name = strings.Trim(name, "-")
		if name != "" {
			return name
		}
	}
	prefix = strings.TrimSpace(prefix)
	if prefix != "" {
		return prefix
	}
	return strings.TrimSpace(fallback)
}

// writeTinyOntologyFixture writes a minimal valid RDFS/OWL document under a
// given namespace, just enough for the ontology import pipeline to parse
// successfully — mirrors the tiny-fixture pattern in
// restore_external_ns_test.go, without the external-namespace wrinkle this
// test doesn't need.
func writeTinyOntologyFixture(t *testing.T, path, namespace, className string) {
	t.Helper()
	content := `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:ex="` + namespace + `"
         xmlns:owl="http://www.w3.org/2002/07/owl#"
         xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
  <owl:Ontology rdf:about="` + namespace + `">
    <owl:versionInfo>1.0</owl:versionInfo>
  </owl:Ontology>
  <rdfs:Class rdf:about="` + namespace + className + `">
    <rdfs:label xml:lang="en">` + className + `</rdfs:label>
  </rdfs:Class>
</rdf:RDF>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// importTinyOntology seeds one ontology + one version (optionally marked
// active) via the same ImportVendoredVersion pipeline the real vendored
// restore path uses, so the DB state matches production shape rather than
// being hand-inserted.
func importTinyOntology(t *testing.T, ctx context.Context, ontologySvc *weaveontology.Service, slug, namespace, className string, active bool) (ontologyID, versionID string) {
	t.Helper()
	ontologyID = weaveontology.GenerateOntologyID(slug)
	versionID = weaveontology.GenerateVersionID(slug, "1.0")

	rdfDir := t.TempDir()
	filename := slug + ".rdf"
	writeTinyOntologyFixture(t, filepath.Join(rdfDir, filename), namespace, className)

	if err := ontologySvc.ImportVendoredVersion(ctx, weaveontology.VendoredOntologyImportRequest{
		OntologyID:    ontologyID,
		VersionID:     versionID,
		VersionString: "1.0",
		Slug:          slug,
		Title:         slug,
		Kind:          "base",
		Namespace:     namespace,
		Prefixes:      []string{slug},
		BaseDir:       rdfDir,
		Files:         []string{filename},
	}); err != nil {
		t.Fatalf("seed ImportVendoredVersion(%s): %v", slug, err)
	}

	if active {
		if err := ontologySvc.SetActiveVersion(ctx, ontologyID, versionID); err != nil {
			t.Fatalf("SetActiveVersion(%s): %v", slug, err)
		}
	}
	return ontologyID, versionID
}

// TestVendorSnapshotIncludesDefaultsOntology: given a DB containing a
// "defaults"-prefixed ontology with an active version carrying rdf_content,
// and a project linked to one ordinary ontology version, writeVendorSnapshot
// (via InitProjectWithOptions(SelfContained: true)) must produce
// vendor/ontologies/<defaults-module>/ alongside the linked ontology. Given a
// DB with NO defaults ontology, vendoring must succeed unchanged (backward
// compatible).
//
// The real-fixture template (internal/testdb.Setup) now always hydrates a
// real "defaults" ontology (prefix "defaults", module w3c-defaults — see
// test/fixtures/*/vendor/ontologies/ontology.pletka.io/w3c-defaults) into
// every clone, since it is a genuine dependency of the AME/LA/ING fixtures.
// "defaults" is globally unique (weave_ontologies_prefix_key), so the first
// subtest below exercises that real ontology instead of seeding a synthetic
// one under the same prefix (which would collide). The second subtest, which
// needs a true no-defaults DB to exercise the backward-compatible path,
// deletes the real defaults ontology from this package's own disposable
// clone first — see the comment there for why that is safe.
func TestVendorSnapshotIncludesDefaultsOntology(t *testing.T) {
	ctx := context.Background()

	t.Run("defaults ontology present is vendored alongside the linked ontology", func(t *testing.T) {
		pool := testdb.Pool(t)
		queries := sqlcgen.New(pool)

		const (
			ownerID   = "DEFAULTS_VENDOR_OWNER_A"
			projectID = "DEFAULTS_VENDOR_PROJECT_A"
		)

		var ordinaryOntologyID, ordinaryVersionID string

		cleanup := func() {
			_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
			_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
			if ordinaryVersionID != "" {
				_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_relations r USING weave_ontology_classes c WHERE r.source_id = c.id AND c.ontology_version_id = $1`, ordinaryVersionID)
				_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_classes WHERE ontology_version_id = $1`, ordinaryVersionID)
				_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE id = $1`, ordinaryVersionID)
			}
			if ordinaryOntologyID != "" {
				_, _ = pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, ordinaryOntologyID)
			}
			_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
		}
		t.Cleanup(func() { cleanup() })

		if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
			ID:          ownerID,
			Type:        "organization",
			DisplayName: "Defaults Vendor Org",
			Slug:        "defaults-vendor-org-a",
			Role:        "",
			Visibility:  "private",
		}); err != nil {
			t.Fatalf("create owner actor: %v", err)
		}

		ontologyStore := weaveontology.NewPostgresStore(pool)
		ontologySvc := weaveontology.NewService(ontologyStore, nil, nil)

		ordinaryOntologyID, ordinaryVersionID = importTinyOntology(t, ctx, ontologySvc, "dvsa-ordinary", "https://example.org/dvsa-ordinary/", "Thing", true)

		// The template clone already carries a real "defaults" ontology (see
		// the TestVendorSnapshotIncludesDefaultsOntology doc comment) — use
		// it rather than seeding a synthetic one under the same prefix.
		defaultsOntology, err := queries.WeaveGetOntologyByPrefix(ctx, "defaults")
		if err != nil {
			t.Fatalf("expected template clone to carry a real defaults ontology: %v", err)
		}
		defaultsVersion, err := queries.WeaveGetActiveOntologyVersion(ctx, defaultsOntology.ID)
		if err != nil {
			t.Fatalf("expected real defaults ontology to have an active version: %v", err)
		}

		uiName, _ := json.Marshal(map[string]string{"en": "Defaults Vendor Project"})
		description, _ := json.Marshal(map[string]string{"en": "Project for TestVendorSnapshotIncludesDefaultsOntology"})
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

		isPrimary := true
		usageNotes := ""
		if _, err := queries.WeaveCreateProjectOntologyVersion(ctx, sqlcgen.WeaveCreateProjectOntologyVersionParams{
			ProjectID:         projectID,
			OntologyVersionID: ordinaryVersionID,
			IsPrimary:         &isPrimary,
			UsageNotes:        &usageNotes,
		}); err != nil {
			t.Fatalf("link project to ordinary ontology version: %v", err)
		}
		// The defaults ontology is deliberately NOT project-linked — it is an
		// implicit ontology-level dependency, never a project requirement.

		sourceDir := t.TempDir()
		mat := gitmaterializer.NewMaterializer(pool, sourceDir, nil)
		if err := mat.InitProjectWithOptions(ctx, projectID, gitmaterializer.InitProjectOptions{SelfContained: true}); err != nil {
			t.Fatalf("InitProjectWithOptions(self-contained): %v", err)
		}
		rootDir := filepath.Join(sourceDir, projectID)

		ordinaryDir := filepath.Join(rootDir, "vendor", "ontologies", "ontology.pletka.io", "dvsa-ordinary", "1.0")
		if _, err := os.Stat(ordinaryDir); err != nil {
			entries, _ := os.ReadDir(filepath.Join(rootDir, "vendor", "ontologies", "ontology.pletka.io"))
			t.Fatalf("expected linked ontology vendored at %s, ontology.pletka.io entries: %v (stat err: %v)", ordinaryDir, entries, err)
		}

		defaultsModule := moduleSlugForOntology(defaultsOntology.Prefix, defaultsOntology.Name, defaultsOntology.ID)
		defaultsDir := filepath.Join(rootDir, "vendor", "ontologies", "ontology.pletka.io", defaultsModule, defaultsVersion.VersionString)
		if _, err := os.Stat(defaultsDir); err != nil {
			entries, _ := os.ReadDir(filepath.Join(rootDir, "vendor", "ontologies", "ontology.pletka.io"))
			t.Fatalf("expected defaults ontology to be vendored implicitly at %s, ontology.pletka.io entries: %v (stat err: %v)", defaultsDir, entries, err)
		}
	})

	t.Run("no defaults ontology in DB vendors unchanged (backward compatible)", func(t *testing.T) {
		pool := testdb.Pool(t)
		queries := sqlcgen.New(pool)

		const (
			ownerID   = "DEFAULTS_VENDOR_OWNER_B"
			projectID = "DEFAULTS_VENDOR_PROJECT_B"
		)

		var ordinaryOntologyID, ordinaryVersionID string

		cleanup := func() {
			_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
			_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
			if ordinaryVersionID != "" {
				_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_relations r USING weave_ontology_classes c WHERE r.source_id = c.id AND c.ontology_version_id = $1`, ordinaryVersionID)
				_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_classes WHERE ontology_version_id = $1`, ordinaryVersionID)
				_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE id = $1`, ordinaryVersionID)
			}
			if ordinaryOntologyID != "" {
				_, _ = pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, ordinaryOntologyID)
			}
			_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
		}
		t.Cleanup(func() { cleanup() })

		// The fixture-hydrated template always carries a real "defaults"
		// ontology now (see the doc comment on the parent test), so exercising
		// the true no-defaults / backward-compatible path requires removing it
		// from this package's own clone first. This is safe and isolated:
		// internal/testdb.Setup cuts one disposable clone per package, and no
		// other test in this package (pkg/service/gitmaterializer) reads or
		// depends on the defaults ontology — this subtest's "present" sibling
		// runs first (t.Run order within the parent Test function) and does
		// not depend on the defaults ontology surviving into this one.
		if defaultsOntology, err := queries.WeaveGetOntologyByPrefix(ctx, "defaults"); err == nil {
			if defaultsVersion, verr := queries.WeaveGetActiveOntologyVersion(ctx, defaultsOntology.ID); verr == nil {
				_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_relations r USING weave_ontology_classes c WHERE r.source_id = c.id AND c.ontology_version_id = $1`, defaultsVersion.ID)
				_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_classes WHERE ontology_version_id = $1`, defaultsVersion.ID)
			} else if !errors.Is(verr, pgx.ErrNoRows) {
				t.Fatalf("look up active version of defaults ontology: %v", verr)
			}
			if _, err := pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE ontology_id = $1`, defaultsOntology.ID); err != nil {
				t.Fatalf("delete defaults ontology versions: %v", err)
			}
			if _, err := pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id = $1`, defaultsOntology.ID); err != nil {
				t.Fatalf("delete defaults ontology: %v", err)
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("look up defaults ontology: %v", err)
		}

		// Guard: confirm the no-defaults path is actually exercised now.
		if _, err := queries.WeaveGetOntologyByPrefix(ctx, "defaults"); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("expected defaults ontology removed from this clone, got err=%v", err)
		}

		if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
			ID:          ownerID,
			Type:        "organization",
			DisplayName: "Defaults Vendor Org B",
			Slug:        "defaults-vendor-org-b",
			Role:        "",
			Visibility:  "private",
		}); err != nil {
			t.Fatalf("create owner actor: %v", err)
		}

		ontologyStore := weaveontology.NewPostgresStore(pool)
		ontologySvc := weaveontology.NewService(ontologyStore, nil, nil)
		ordinaryOntologyID, ordinaryVersionID = importTinyOntology(t, ctx, ontologySvc, "dvsb-ordinary", "https://example.org/dvsb-ordinary/", "Thing", true)

		uiName, _ := json.Marshal(map[string]string{"en": "Defaults Vendor Project B"})
		description, _ := json.Marshal(map[string]string{"en": "Project for TestVendorSnapshotIncludesDefaultsOntology/no-defaults"})
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

		isPrimary := true
		usageNotes := ""
		if _, err := queries.WeaveCreateProjectOntologyVersion(ctx, sqlcgen.WeaveCreateProjectOntologyVersionParams{
			ProjectID:         projectID,
			OntologyVersionID: ordinaryVersionID,
			IsPrimary:         &isPrimary,
			UsageNotes:        &usageNotes,
		}); err != nil {
			t.Fatalf("link project to ordinary ontology version: %v", err)
		}

		sourceDir := t.TempDir()
		mat := gitmaterializer.NewMaterializer(pool, sourceDir, nil)
		if err := mat.InitProjectWithOptions(ctx, projectID, gitmaterializer.InitProjectOptions{SelfContained: true}); err != nil {
			t.Fatalf("InitProjectWithOptions(self-contained), no defaults ontology in DB: %v", err)
		}
		rootDir := filepath.Join(sourceDir, projectID)

		ordinaryDir := filepath.Join(rootDir, "vendor", "ontologies", "ontology.pletka.io", "dvsb-ordinary", "1.0")
		if _, err := os.Stat(ordinaryDir); err != nil {
			t.Fatalf("expected linked ontology still vendored without a defaults ontology present: %v", err)
		}
		if _, err := os.Stat(filepath.Join(rootDir, "vendor", "ontologies", "ontology.pletka.io", "defaults")); err == nil {
			t.Fatal("expected no defaults vendor dir when no defaults ontology exists in the DB")
		}
	})
}

// TestImportVendoredVersion_ActivatesFirstImportedVersion verifies the fix
// for the "restored databases never have an active defaults version" review
// finding: ImportVendoredVersion must set the just-imported version active
// when the ontology has no active version yet. Without this, every vendored
// ontology (import_builder.go's CreateVersionInput always sets
// IsActive: false) lands permanently inactive after a restore, and
// pathaudit's resolveDefaultsVersionID finds nothing to resolve standard
// terms against.
func TestImportVendoredVersion_ActivatesFirstImportedVersion(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	ontologyStore := weaveontology.NewPostgresStore(pool)
	ontologySvc := weaveontology.NewService(ontologyStore, nil, nil)

	ontologyID, versionID := importTinyOntology(t, ctx, ontologySvc, "activate-first-test", "https://example.org/activate-first/", "Thing", false)
	t.Cleanup(func() {
		_ = ontologySvc.DeleteVersion(context.Background(), versionID)
		_ = ontologySvc.DeleteOntology(context.Background(), ontologyID)
	})

	active, err := ontologySvc.GetActiveVersion(ctx, ontologyID)
	if err != nil {
		t.Fatalf("GetActiveVersion: %v", err)
	}
	if active.ID != versionID {
		t.Fatalf("active version = %s, want %s (the just-imported version, since the ontology had none active)", active.ID, versionID)
	}
}

// TestImportVendoredVersion_DoesNotOverrideExistingActiveVersion verifies the
// fix's idempotency guard: importing a second version for an ontology that
// already has an active version must NOT flip activation onto the new
// version — an admin's (or an earlier restore's) choice of active version is
// preserved. The fix only activates when no active version currently exists.
func TestImportVendoredVersion_DoesNotOverrideExistingActiveVersion(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	ontologyStore := weaveontology.NewPostgresStore(pool)
	ontologySvc := weaveontology.NewService(ontologyStore, nil, nil)

	slug := "activate-guard-test"
	namespace := "https://example.org/activate-guard/"
	ontologyID, firstVersionID := importTinyOntology(t, ctx, ontologySvc, slug, namespace, "Thing", false)

	rdfDir := t.TempDir()
	secondVersionID := weaveontology.GenerateVersionID(slug, "2.0")
	filename := slug + "-v2.rdf"
	writeTinyOntologyFixture(t, filepath.Join(rdfDir, filename), namespace, "Actor")
	if err := ontologySvc.ImportVendoredVersion(ctx, weaveontology.VendoredOntologyImportRequest{
		OntologyID:    ontologyID,
		VersionID:     secondVersionID,
		VersionString: "2.0",
		Slug:          slug,
		Title:         slug,
		Kind:          "base",
		Namespace:     namespace,
		Prefixes:      []string{slug},
		BaseDir:       rdfDir,
		Files:         []string{filename},
	}); err != nil {
		t.Fatalf("import second version: %v", err)
	}
	t.Cleanup(func() {
		_ = ontologySvc.DeleteVersion(context.Background(), secondVersionID)
		_ = ontologySvc.DeleteVersion(context.Background(), firstVersionID)
		_ = ontologySvc.DeleteOntology(context.Background(), ontologyID)
	})

	active, err := ontologySvc.GetActiveVersion(ctx, ontologyID)
	if err != nil {
		t.Fatalf("GetActiveVersion: %v", err)
	}
	if active.ID != firstVersionID {
		t.Fatalf("active version = %s, want %s (first-imported version; second import must not steal activation)", active.ID, firstVersionID)
	}
}
