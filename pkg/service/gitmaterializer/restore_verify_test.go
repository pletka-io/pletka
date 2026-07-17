package gitmaterializer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildVerifyFixture creates a temp vendor tree with one vendored project and
// one vendored ontology, hashes both with hashDirectoryTree, and returns a
// ProjectSnapshot whose Sum entries match those hashes. Callers can tamper
// with the returned directories or mutate snapshot.Sum to exercise failure
// paths.
func buildVerifyFixture(t *testing.T) (snapshot *ProjectSnapshot, ontologyDir, projectDir string) {
	t.Helper()
	root := t.TempDir()

	ontologyDir = filepath.Join(root, "vendor", "ontologies", "example.org", "ontology", "core", "1.0.0")
	if err := os.MkdirAll(filepath.Join(ontologyDir, "src"), 0o755); err != nil {
		t.Fatalf("mkdir ontology dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ontologyDir, "ontology.yaml"), []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatalf("write ontology.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ontologyDir, "src", "concept.ttl"), []byte("@prefix ex: <http://example.org/> .\n"), 0o644); err != nil {
		t.Fatalf("write ontology source: %v", err)
	}
	ontologyHash, err := hashDirectoryTree(ontologyDir)
	if err != nil {
		t.Fatalf("hash ontology dir: %v", err)
	}

	projectDir = filepath.Join(root, "vendor", "projects", "example.org", "parent-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "project.yaml"), []byte("project:\n  id: parent\n"), 0o644); err != nil {
		t.Fatalf("write project.yaml: %v", err)
	}
	projectHash, err := hashDirectoryTree(projectDir)
	if err != nil {
		t.Fatalf("hash project dir: %v", err)
	}

	snapshot = &ProjectSnapshot{
		RootDir: root,
		Sum: []PletkaSumEntry{
			{Module: "example.org/parent-project", Version: "draft", TreeSHA: projectHash},
			{Module: "example.org/ontology/core", Version: "1.0.0", TreeSHA: ontologyHash},
		},
		Vendor: VendorSnapshotSet{
			Projects: []VendoredProjectSnapshot{
				{
					Module:   "example.org/parent-project",
					Snapshot: &ProjectSnapshot{RootDir: projectDir},
				},
			},
			Ontologies: []VendoredOntologySnapshot{
				{
					Module:   "example.org/ontology/core",
					Version:  "1.0.0",
					Snapshot: &OntologySnapshot{RootDir: ontologyDir},
				},
			},
		},
	}
	return snapshot, ontologyDir, projectDir
}

func TestVerifyVendorChecksums(t *testing.T) {
	t.Run("matching ontology and project checksums pass", func(t *testing.T) {
		snapshot, _, _ := buildVerifyFixture(t)

		if err := verifyVendorChecksums(snapshot); err != nil {
			t.Fatalf("verifyVendorChecksums() = %v, want nil", err)
		}
	})

	t.Run("tampered ontology file fails naming the module", func(t *testing.T) {
		snapshot, ontologyDir, _ := buildVerifyFixture(t)
		if err := os.WriteFile(filepath.Join(ontologyDir, "src", "concept.ttl"), []byte("tampered"), 0o644); err != nil {
			t.Fatalf("tamper ontology source: %v", err)
		}

		err := verifyVendorChecksums(snapshot)
		if err == nil {
			t.Fatal("verifyVendorChecksums() = nil, want error")
		}
		if !strings.Contains(err.Error(), "example.org/ontology/core") {
			t.Fatalf("error %q does not name the tampered module", err.Error())
		}
	})

	t.Run("tampered project file fails naming the module", func(t *testing.T) {
		snapshot, _, projectDir := buildVerifyFixture(t)
		if err := os.WriteFile(filepath.Join(projectDir, "project.yaml"), []byte("project:\n  id: tampered\n"), 0o644); err != nil {
			t.Fatalf("tamper project.yaml: %v", err)
		}

		err := verifyVendorChecksums(snapshot)
		if err == nil {
			t.Fatal("verifyVendorChecksums() = nil, want error")
		}
		if !strings.Contains(err.Error(), "example.org/parent-project") {
			t.Fatalf("error %q does not name the tampered module", err.Error())
		}
	})

	t.Run("vendored ontology with no sum entry fails", func(t *testing.T) {
		snapshot, _, _ := buildVerifyFixture(t)
		snapshot.Sum = snapshot.Sum[:1] // drop the ontology's pletka.sum entry, keep the project's

		err := verifyVendorChecksums(snapshot)
		if err == nil {
			t.Fatal("verifyVendorChecksums() = nil, want error")
		}
		if !strings.Contains(err.Error(), "example.org/ontology/core") {
			t.Fatalf("error %q does not name the missing module", err.Error())
		}
	})

	t.Run("two pinned versions of one project module pair correctly", func(t *testing.T) {
		root := t.TempDir()

		writeVersionedProjectDir := func(version string) (dir, hash string) {
			dir = filepath.Join(root, "vendor", "projects", "example.org", "shared-project", version)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir project dir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "project.yaml"), []byte("project:\n  id: "+version+"\n"), 0o644); err != nil {
				t.Fatalf("write project.yaml: %v", err)
			}
			h, err := hashDirectoryTree(dir)
			if err != nil {
				t.Fatalf("hash project dir: %v", err)
			}
			return dir, h
		}

		v1Dir, v1Hash := writeVersionedProjectDir("v1.0.0")
		v2Dir, v2Hash := writeVersionedProjectDir("v2.0.0")
		if v1Hash == v2Hash {
			t.Fatal("expected distinct hashes for v1 and v2 fixtures")
		}

		snapshot := &ProjectSnapshot{
			RootDir: root,
			Sum: []PletkaSumEntry{
				{Module: "example.org/shared-project", Version: "v1.0.0", TreeSHA: v1Hash},
				{Module: "example.org/shared-project", Version: "v2.0.0", TreeSHA: v2Hash},
			},
			Vendor: VendorSnapshotSet{
				Projects: []VendoredProjectSnapshot{
					{
						Module:   "example.org/shared-project",
						Version:  "v1.0.0",
						Snapshot: &ProjectSnapshot{RootDir: v1Dir},
					},
					{
						Module:   "example.org/shared-project",
						Version:  "v2.0.0",
						Snapshot: &ProjectSnapshot{RootDir: v2Dir},
					},
				},
			},
		}

		if err := verifyVendorChecksums(snapshot); err != nil {
			t.Fatalf("verifyVendorChecksums() = %v, want nil (correct version pairing)", err)
		}
	})

	t.Run("draft version lookup with multiple sum entries for the module is ambiguous", func(t *testing.T) {
		snapshot, _, _ := buildVerifyFixture(t)
		// buildVerifyFixture's project dependency has Version "" (draft: no
		// version subdirectory). A second, unrelated version entry for the
		// same module makes the version-less wildcard lookup ambiguous.
		snapshot.Sum = append(snapshot.Sum, PletkaSumEntry{
			Module:  "example.org/parent-project",
			Version: "v1.0.0",
			TreeSHA: "deadbeef",
		})

		err := verifyVendorChecksums(snapshot)
		if err == nil {
			t.Fatal("verifyVendorChecksums() = nil, want error")
		}
		if !strings.Contains(err.Error(), "ambiguous entries for module example.org/parent-project") {
			t.Fatalf("error %q does not describe the ambiguity", err.Error())
		}
	})
}
