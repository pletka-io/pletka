package gitmaterializer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
)

const legacyOntologyFile = "ontology-snapshot.json"

// Project schema 2 transports preserved evidence, independently of a complete
// project restore certificate. Bodies are keyed inside this file by concrete ID,
// so two revisions with the same human version label cannot share a vendor path.
type legacyOntologyManifest struct {
	File    string `json:"file"`
	Digest  string `json:"digest"`
	Version string `json:"version"`
	value   ontologysnapshot.Artifact
}

func legacyOntologyRef(version string, a ontologysnapshot.Artifact) *legacyOntologyManifest {
	return &legacyOntologyManifest{File: legacyOntologyFile, Digest: a.Digest, Version: version, value: a}
}

func legacySnapshotBodies(a ontologysnapshot.Artifact) (map[string]ontologysnapshot.Body, error) {
	d, err := a.Decode()
	if err != nil {
		return nil, err
	}
	out := map[string]ontologysnapshot.Body{}
	add := func(d ontologysnapshot.Document) error {
		for _, b := range d.Inventory {
			if old, ok := out[b.VersionID]; ok && (old.BodyJSON != b.BodyJSON || old.Digest != b.Digest || old.OntologyID != b.OntologyID) {
				return fmt.Errorf("conflicting legacy body identity %s", b.VersionID)
			}
			out[b.VersionID] = b
		}
		return nil
	}
	if err := add(d); err != nil {
		return nil, err
	}
	for _, s := range d.Sources {
		var source ontologysnapshot.Document
		if err := json.Unmarshal([]byte(s.JSON), &source); err != nil {
			return nil, err
		}
		if err := add(source); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func loadLegacyOntology(snapshot *ProjectSnapshot) error {
	manifest, mod := snapshot.Manifest, snapshot.Mod
	if manifest.SchemaVersion != 1 && manifest.SchemaVersion != 2 {
		return fmt.Errorf("unsupported project schema %d", manifest.SchemaVersion)
	}
	if mod.SchemaVersion != 1 && mod.SchemaVersion != 2 {
		return fmt.Errorf("unsupported module schema %d", mod.SchemaVersion)
	}
	path := filepath.Join(snapshot.RootDir, legacyOntologyFile)
	info, err := os.Lstat(path)
	if manifest.SchemaVersion == 1 {
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil || manifest.LegacyOntology != nil || mod.LegacyOntology != nil || mod.SchemaVersion != 1 {
			return fmt.Errorf("legacy evidence requires project and module schema 2")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("legacy ontology snapshot must be a regular file")
	}
	ref := manifest.LegacyOntology
	if ref == nil || ref.File != legacyOntologyFile {
		return fmt.Errorf("legacy manifest requires exact snapshot file and digest")
	}
	data, err := os.ReadFile(path) //nolint:gosec // Fixed filename under the selected restore root; symlinks rejected above.
	if err != nil {
		return err
	}
	a := ontologysnapshot.Artifact{JSON: string(data), Digest: ref.Digest}
	snapshot.LegacyOntology = &a
	return validateLegacyOntology(snapshot)
}

func validateLegacyOntology(s *ProjectSnapshot) error {
	if err := validateLegacyFormat(s); err != nil {
		return err
	}
	if s.Mod == nil || s.Manifest.SchemaVersion != 2 {
		return nil
	}
	if s.Manifest.Namespaces != nil {
		return fmt.Errorf("legacy schema 2 must omit namespaces; the exact snapshot retains the alias registry")
	}
	ref, mod := s.Manifest.LegacyOntology, s.Mod.LegacyOntology
	if ref == nil || mod == nil || s.Mod.SchemaVersion != 2 || s.LegacyOntology == nil || ref.File != legacyOntologyFile || ref.File != mod.File || ref.Digest != mod.Digest || ref.Version != mod.Version || ref.Digest != s.LegacyOntology.Digest {
		return fmt.Errorf("legacy snapshot manifest identity mismatch")
	}
	d, err := s.LegacyOntology.Decode()
	if err != nil {
		return err
	}
	if d.ProjectID != s.Manifest.Project.ID || d.ProjectID != s.Mod.Module.ProjectID || d.Version != ref.Version {
		return fmt.Errorf("legacy snapshot project/release identity mismatch")
	}
	if err := validateLegacyLinks(s, d); err != nil {
		return err
	}
	return validateLegacyRequirements(s.Mod.Require.Ontologies, *s.LegacyOntology)
}

func validateLegacyFormat(s *ProjectSnapshot) error {
	if s.Manifest.SchemaVersion < 0 || s.Manifest.SchemaVersion > 2 {
		return fmt.Errorf("unsupported project schema %d", s.Manifest.SchemaVersion)
	}
	if s.Mod != nil && (s.Mod.SchemaVersion < 0 || s.Mod.SchemaVersion > 2) {
		return fmt.Errorf("unsupported module schema %d", s.Mod.SchemaVersion)
	}
	if s.Mod == nil {
		if s.Manifest.SchemaVersion == 2 || s.LegacyOntology != nil || s.Manifest.LegacyOntology != nil {
			return fmt.Errorf("project module manifest missing")
		}
		return nil
	}
	if s.Manifest.SchemaVersion != 2 {
		if s.LegacyOntology != nil || s.Manifest.LegacyOntology != nil || s.Mod.LegacyOntology != nil || s.Mod.SchemaVersion == 2 {
			return fmt.Errorf("legacy evidence requires project and module schema 2")
		}
		for _, r := range s.Mod.Require.Ontologies {
			if r.BodyDigest != "" {
				return fmt.Errorf("legacy body requires captured project evidence")
			}
		}
		return nil
	}
	return nil
}

func validateLegacyLinks(s *ProjectSnapshot, d ontologysnapshot.Document) error {
	var links []projectManifestLinkedOntology
	for _, b := range d.Inventory {
		var content struct {
			Version struct {
				Label string `json:"version_string"`
			}
		}
		var selection struct {
			Primary bool `json:"is_primary"`
		}
		if err := json.Unmarshal([]byte(b.BodyJSON), &content); err != nil {
			return err
		}
		if err := json.Unmarshal(b.Selection, &selection); err != nil {
			return err
		}
		links = append(links, projectManifestLinkedOntology{OntologyID: b.OntologyID, OntologyVersionID: b.VersionID, Version: content.Version.Label, IsPrimary: selection.Primary})
	}
	var actual []projectManifestLinkedOntology
	if s.Manifest.Ontologies != nil {
		actual = append(actual, s.Manifest.Ontologies.LinkedVersions...)
	}
	sortLinks := func(v []projectManifestLinkedOntology) {
		sort.Slice(v, func(i, j int) bool { return v[i].OntologyVersionID < v[j].OntologyVersionID })
	}
	sortLinks(links)
	sortLinks(actual)
	if !reflect.DeepEqual(links, actual) {
		return fmt.Errorf("legacy ontology selections differ from captured evidence")
	}
	return nil
}

func validateLegacyRequirements(requirements []pletkaOntologyRequirement, a ontologysnapshot.Artifact) error {
	bodies, err := legacySnapshotBodies(a)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, r := range requirements {
		b, ok := bodies[r.OntologyVersionID]
		var content struct {
			Version struct {
				Label string `json:"version_string"`
			}
		}
		if ok {
			if err := json.Unmarshal([]byte(b.BodyJSON), &content); err != nil {
				return err
			}
		}
		if !ok || seen[r.OntologyVersionID] || r.Digest != "" || r.Artifact != nil || r.BodyDigest != b.Digest || r.OntologyID != b.OntologyID || r.Version != content.Version.Label || r.Module == "" {
			return fmt.Errorf("legacy ontology requirement differs from captured body %s", r.OntologyVersionID)
		}
		seen[r.OntologyVersionID] = true
	}
	if len(seen) != len(bodies) {
		return fmt.Errorf("legacy ontology requirements omit captured bodies")
	}
	return nil
}

func legacyOntologyBundle(snapshot *ProjectSnapshot) ([]ontologysnapshot.Artifact, error) {
	projects := []*ProjectSnapshot{snapshot}
	vendors := map[string]VendoredProjectSnapshot{}
	legacyProjects := map[string]bool{}
	for _, p := range snapshot.Vendor.Projects {
		if p.Snapshot == nil {
			return nil, fmt.Errorf("missing project snapshot")
		}
		if p.Snapshot.LegacyOntology != nil {
			legacyProjects[p.Snapshot.Manifest.Project.ID] = true
			ref := p.Snapshot.Manifest.LegacyOntology
			if ref == nil || p.Version != ref.Version || p.Snapshot.Mod == nil || p.Module != p.Snapshot.Mod.Module.Path {
				return nil, fmt.Errorf("vendored legacy evidence differs from selected module/release")
			}
		}
		id := p.Snapshot.Manifest.Project.ID
		key := id + "@" + p.Version
		if _, ok := vendors[key]; ok {
			return nil, fmt.Errorf("duplicate vendored project identity %s", key)
		}
		vendors[key] = p
		projects = append(projects, p.Snapshot)
	}
	var out []ontologysnapshot.Artifact
	for _, p := range projects {
		if p == nil {
			return nil, fmt.Errorf("missing project snapshot")
		}
		if err := validateLegacyOntology(p); err != nil {
			return nil, err
		}

		if p.LegacyOntology != nil {
			out = append(out, *p.LegacyOntology)
		}
	}
	if err := validateLegacyProjectSelectors(projects, vendors, legacyProjects); err != nil {
		return nil, err
	}
	if err := ontologysnapshot.ValidateBundle(out); err != nil {
		return nil, err
	}
	return out, nil
}

func validateLegacyProjectSelectors(projects []*ProjectSnapshot, vendors map[string]VendoredProjectSnapshot, legacyProjects map[string]bool) error {
	selectedLegacy := map[string]bool{}
	for _, p := range projects {
		if p.Mod != nil {
			for _, req := range p.Mod.Require.Projects {
				dep, ok := vendors[req.ProjectID+"@"+req.Version]
				// Enforce captured selectors for every legacy consumer and every
				// legacy vendor selected by an older-format consumer.
				if p.LegacyOntology != nil || legacyProjects[req.ProjectID] {
					if !ok || dep.Module != req.Module {
						return fmt.Errorf("legacy project dependency %s@%s is missing or mismatched", req.ProjectID, req.Version)
					}
					selectedLegacy[req.ProjectID+"@"+req.Version] = true
				}
			}
		}
	}
	for key, p := range vendors {
		if p.Snapshot.LegacyOntology != nil && !selectedLegacy[key] {
			return fmt.Errorf("legacy project vendor has no selected release: %s", key)
		}
	}
	return nil
}
