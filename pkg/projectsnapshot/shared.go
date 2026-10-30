package projectsnapshot

import "fmt"

// A concrete ontology version has one body/module across project contexts.
// Context-specific namespace aliases and semantic assessments remain separate.
func (g *graph) checkSharedOntologies() error {
	type identity struct{ owner, digest, module string }
	seen := map[string]identity{}
	publications := map[string]string{}
	for _, id := range g.identities() {
		e := g.projects[id].evidence
		check := func(version, owner, digest string) error {
			module, err := e.descriptors.OntologyModule(version)
			if err != nil {
				return err
			}
			value := identity{owner, digest, module}
			if old, ok := seen[version]; ok && old != value {
				return fmt.Errorf("conflicting retained ontology body or module for %s", version)
			}
			seen[version] = value
			return nil
		}
		if e.contract == 2 {
			d, err := e.artifact.Decode()
			if err != nil {
				return err
			}
			for _, body := range d.Inventory {
				if err := check(body.VersionID, body.OntologyID, body.Digest); err != nil {
					return err
				}
			}
			continue
		}
		for _, publication := range e.publications {
			d, err := publication.Artifact.Decode()
			if err != nil {
				return err
			}
			if err := check(d.VersionID, d.OntologyID, d.BodyDigest); err != nil {
				return err
			}
			if previous, ok := publications[d.VersionID]; ok && previous != publication.Artifact.Digest {
				return fmt.Errorf("conflicting retained publication for %s", d.VersionID)
			}
			publications[d.VersionID] = publication.Artifact.Digest
		}
	}
	return nil
}
