package ontology

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
)

// OfflineVersionSpec names one ontology file to load without a database.
type OfflineVersionSpec struct {
	Ontology   *domain.Ontology
	VersionID  string
	Version    string
	BaseDir    string
	File       string
	Companions []string
	Bindings   []NamespaceBinding
}

// LoadOfflineVersion parses the RDF file through the same import builder the
// database import uses and returns the rows the autocomplete index is built
// from. Qnames are computed as prefix:local, the way the database does.
func LoadOfflineVersion(spec OfflineVersionSpec) (autocomplete.MemoryVersion, error) {
	if spec.Ontology == nil {
		return autocomplete.MemoryVersion{}, fmt.Errorf("offline version: ontology is required")
	}
	companions := make([]ImportCompanionFile, 0, len(spec.Companions))
	for _, c := range spec.Companions {
		companions = append(companions, ImportCompanionFile{File: c})
	}
	res, err := BuildImportVersionInputFromFile(ImportVersionFileOptions{
		BaseDir:       spec.BaseDir,
		File:          spec.File,
		Ontology:      spec.Ontology,
		VersionID:     spec.VersionID,
		VersionString: spec.Version,
		// The database import sees the standard prefixes (xsd, rdfs, owl, ...)
		// as global bindings; offline they have to be supplied here.
		NamespaceBindings: MergeNamespaceBindings(spec.Bindings, StandardNamespaceBindings()),
		Companions:        companions,
	})
	if err != nil {
		return autocomplete.MemoryVersion{}, fmt.Errorf("offline version %s %s: %w", spec.Ontology.Prefix, spec.Version, err)
	}
	in := res.Input
	qnameByID := make(map[string]string, len(in.Classes)+len(in.Properties))
	mv := autocomplete.MemoryVersion{
		Version:    &domain.OntologyVersion{ID: spec.VersionID, OntologyID: spec.Ontology.ID, VersionString: spec.Version},
		Ontology:   spec.Ontology,
		Classes:    make([]*domain.OntologyClass, 0, len(in.Classes)),
		Properties: make([]*domain.OntologyProperty, 0, len(in.Properties)),
		Relations:  make([]domain.OntologyRelationWithSource, 0, len(in.Relations)),
	}
	for _, c := range in.Classes {
		q := c.Prefix + ":" + c.LocalName
		qnameByID[c.ID] = q
		mv.Classes = append(mv.Classes, &domain.OntologyClass{
			ID: c.ID, OntologyVersionID: spec.VersionID, Prefix: c.Prefix, LocalName: c.LocalName,
			URI: c.URI, Qname: q, Label: c.Label, Comment: c.Comment,
		})
	}
	for _, p := range in.Properties {
		q := p.Prefix + ":" + p.LocalName
		qnameByID[p.ID] = q
		mv.Properties = append(mv.Properties, &domain.OntologyProperty{
			ID: p.ID, OntologyVersionID: spec.VersionID, Prefix: p.Prefix, LocalName: p.LocalName,
			URI: p.URI, Qname: q, Label: p.Label, Comment: p.Comment, PropertyType: p.PropertyType,
			IsFunctional: p.IsFunctional, IsInverseFunctional: p.IsInverseFunctional, IsTransitive: p.IsTransitive,
			IsSymmetric: p.IsSymmetric, IsAsymmetric: p.IsAsymmetric, IsReflexive: p.IsReflexive, IsIrreflexive: p.IsIrreflexive,
			InversePropertyURI: p.InversePropertyURI,
		})
	}
	for _, r := range in.Relations {
		mv.Relations = append(mv.Relations, domain.OntologyRelationWithSource{OntologyRelation: r, SourceQname: qnameByID[r.SourceID]})
	}
	return mv, nil
}
