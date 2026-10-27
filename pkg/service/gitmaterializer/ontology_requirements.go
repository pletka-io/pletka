package gitmaterializer

import "fmt"

// Verify the consumer's identities as well as each vendor's internal integrity.
// Legacy requirements have no digest; they must never select a published vendor.
func validateOntologyRequirements(snapshot *ProjectSnapshot) error {
	byVersion := map[string]VendoredOntologySnapshot{}
	for _, dep := range snapshot.Vendor.Ontologies {
		if dep.Snapshot == nil {
			return fmt.Errorf("ontology vendor snapshot is missing")
		}
		id := dep.Snapshot.Manifest.Ontology.VersionID
		if _, ok := byVersion[id]; ok {
			return fmt.Errorf("duplicate ontology vendor version %s", id)
		}
		byVersion[id] = dep
	}
	projects := []*ProjectSnapshot{snapshot}
	for _, dep := range snapshot.Vendor.Projects {
		if dep.Snapshot == nil {
			return fmt.Errorf("project vendor snapshot is missing")
		}
		projects = append(projects, dep.Snapshot)
	}
	selected := map[string]bool{}
	for _, project := range projects {
		if project.Mod == nil {
			continue
		}
		for _, req := range project.Mod.Require.Ontologies {
			dep, exists := byVersion[req.OntologyVersionID]
			if err := validateOntologyRequirement(req, dep, exists); err != nil {
				return err
			}
			selected[req.OntologyVersionID] = true
		}
	}
	for id, dep := range byVersion {
		if dep.Snapshot.Artifact != nil && !selected[id] {
			return fmt.Errorf("published ontology vendor %s has no pinned requirement", id)
		}
	}
	return nil
}

func validateOntologyRequirement(req pletkaOntologyRequirement, dep VendoredOntologySnapshot, exists bool) error {
	if req.Digest == "" {
		if exists && dep.Snapshot.Artifact != nil {
			return fmt.Errorf("published ontology requirement %s has no artifact digest", req.OntologyVersionID)
		}
		return nil // Old snapshots retain their explicit unverified path.
	}
	if !exists || dep.Snapshot.Artifact == nil {
		return fmt.Errorf("published ontology requirement %s has no artifact vendor", req.OntologyVersionID)
	}
	root := dep.Snapshot.Manifest.Ontology
	if req.Digest != dep.Snapshot.Artifact.Digest || req.OntologyID != root.OntologyID || req.Version != root.Version || req.Module != root.Module || req.Module != dep.Module || vendorVersionDir(req.Version, req.OntologyVersionID) != dep.Version {
		return fmt.Errorf("ontology requirement %s differs from its published vendor identity", req.OntologyVersionID)
	}
	return nil
}
