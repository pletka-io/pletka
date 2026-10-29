package gitmaterializer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
)

func legacyTransportFixture(t *testing.T) *ProjectSnapshot {
	t.Helper()
	hash := func(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
	d := ontologysnapshot.Document{FormatVersion: 1, ProjectID: "project", Version: "1.0.0"}
	links := make([]projectManifestLinkedOntology, 0, 2)
	reqs := make([]pletkaOntologyRequirement, 0, 2)
	for _, id := range []string{"martin-revision", "original-revision"} {
		body := fmt.Sprintf(`{"format_version":1,"ontology":{"id":"crm"},"version":{"id":%q,"ontology_id":"crm","version_string":"7.1.3"},"classes":[]}`, id)
		selection := json.RawMessage(fmt.Sprintf(`{"project_id":"project","version_number":"1.0.0","ontology_version_id":%q,"is_primary":false}`, id))
		d.Inventory = append(d.Inventory, ontologysnapshot.Body{OntologyID: "crm", VersionID: id, BodyJSON: body, Digest: hash(body), Selection: selection})
		links = append(links, projectManifestLinkedOntology{OntologyID: "crm", OntologyVersionID: id, Version: "7.1.3"})
		reqs = append(reqs, pletkaOntologyRequirement{Module: "example.org/crm", OntologyID: "crm", OntologyVersionID: id, Version: "7.1.3", BodyDigest: hash(body)})
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	a := ontologysnapshot.Artifact{JSON: string(data), Digest: hash(string(data))}
	return &ProjectSnapshot{RootDir: t.TempDir(), LegacyOntology: &a,
		Manifest: projectManifest{SchemaVersion: 2, Project: projectManifestProject{ID: "project"}, LegacyOntology: legacyOntologyRef("1.0.0", a), Ontologies: &projectManifestOntologies{LinkedVersions: links}},
		Mod:      &pletkaModManifest{SchemaVersion: 2, Module: pletkaModModule{ProjectID: "project"}, LegacyOntology: legacyOntologyRef("1.0.0", a), Require: pletkaModRequire{Ontologies: reqs}},
	}
}

func TestLegacyTransportRetainsSameLabelRevisions(t *testing.T) {
	s := legacyTransportFixture(t)
	if err := validateOntologyRequirements(s); err != nil {
		t.Fatal(err)
	}
	b, err := legacySnapshotBodies(*s.LegacyOntology)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 2 {
		t.Fatal("same upstream label discarded a concrete revision")
	}
	if err := writeEntityFile(s.RootDir, legacyOntologyFile, []byte(s.LegacyOntology.JSON)); err != nil {
		t.Fatal(err)
	}
	want := *s.LegacyOntology
	s.LegacyOntology = nil
	if err := loadLegacyOntology(s); err != nil {
		t.Fatal(err)
	}
	if *s.LegacyOntology != want {
		t.Fatal("filesystem load changed exact evidence")
	}
}

func TestLegacyTransportRejectsFormatAndInventoryDrift(t *testing.T) {
	for name, change := range map[string]func(*ProjectSnapshot){
		"unknown project format":    func(s *ProjectSnapshot) { s.Manifest.SchemaVersion = 3 },
		"mismatched module format":  func(s *ProjectSnapshot) { s.Mod.SchemaVersion = 1 },
		"lost evidence":             func(s *ProjectSnapshot) { s.LegacyOntology = nil },
		"lost requirement":          func(s *ProjectSnapshot) { s.Mod.Require.Ontologies = s.Mod.Require.Ontologies[:1] },
		"duplicate requirement":     func(s *ProjectSnapshot) { s.Mod.Require.Ontologies[1] = s.Mod.Require.Ontologies[0] },
		"publication impersonation": func(s *ProjectSnapshot) { s.Mod.Require.Ontologies[0].Digest = s.Mod.Require.Ontologies[0].BodyDigest },
		"changed primary":           func(s *ProjectSnapshot) { s.Manifest.Ontologies.LinkedVersions[0].IsPrimary = true },
		"other project":             func(s *ProjectSnapshot) { s.Manifest.Project.ID = "other" },
		"other release":             func(s *ProjectSnapshot) { s.Mod.LegacyOntology.Version = "2.0.0" },
		"unsafe path":               func(s *ProjectSnapshot) { s.Manifest.LegacyOntology.File = "../elsewhere" },
	} {
		t.Run(name, func(t *testing.T) {
			s := legacyTransportFixture(t)
			change(s)
			if err := validateOntologyRequirements(s); err == nil {
				t.Fatal("invalid transport accepted")
			}
		})
	}
}

func TestLegacyTransportRejectsStrippedManifestAndSymlink(t *testing.T) {
	s := legacyTransportFixture(t)
	if err := writeEntityFile(s.RootDir, legacyOntologyFile, []byte(s.LegacyOntology.JSON)); err != nil {
		t.Fatal(err)
	}
	s.Manifest.SchemaVersion = 1
	s.Mod.SchemaVersion = 1
	s.Manifest.LegacyOntology = nil
	s.Mod.LegacyOntology = nil
	if err := loadLegacyOntology(s); err == nil {
		t.Fatal("stripped evidence marker accepted")
	}
	s = legacyTransportFixture(t)
	outside := filepath.Join(t.TempDir(), "evidence")
	if err := os.WriteFile(outside, []byte(s.LegacyOntology.JSON), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.RootDir, legacyOntologyFile)); err != nil {
		t.Fatal(err)
	}
	if err := loadLegacyOntology(s); err == nil {
		t.Fatal("snapshot symlink accepted")
	}
}

func TestLegacyVendorBindsConsumerAndDirectoryVersion(t *testing.T) {
	for _, mode := range []string{"valid", "wrong-directory", "wrong-selector", "missing-selector", "unknown-format"} {
		t.Run(mode, func(t *testing.T) {
			child := legacyTransportFixture(t)
			child.Mod.Module.Path = "example.org/project"
			root := &ProjectSnapshot{
				Manifest: projectManifest{SchemaVersion: 1},
				Mod:      &pletkaModManifest{SchemaVersion: 1, Require: pletkaModRequire{Projects: []pletkaProjectRequirement{{ProjectID: "project", Module: "example.org/project", Version: "1.0.0"}}}},
				Vendor:   VendorSnapshotSet{Projects: []VendoredProjectSnapshot{{Module: "example.org/project", Version: "1.0.0", Snapshot: child}}},
			}
			switch mode {
			case "wrong-directory":
				root.Vendor.Projects[0].Version = "2.0.0"
			case "wrong-selector":
				root.Mod.Require.Projects[0].Version = "2.0.0"
			case "missing-selector":
				root.Mod.Require.Projects = nil
			case "unknown-format":
				root.Manifest.SchemaVersion = 3
				root.Mod.SchemaVersion = 3
				root.Vendor = VendorSnapshotSet{}
			}
			err := validateOntologyRequirements(root)
			if (mode == "valid") != (err == nil) {
				t.Fatalf("validation: %v", err)
			}
		})
	}
}
