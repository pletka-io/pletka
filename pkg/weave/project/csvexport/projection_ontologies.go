package csvexport

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// csvOntology reads only retained body columns. Active policy is deliberately
// absent from current publication bodies; nil means unknown, not false.
type csvOntology struct {
	Ontology struct{ Name string }
	Version  struct {
		ID         string
		Version    string              `json:"version_string"`
		URI        string              `json:"ontology_uri"`
		IRI        string              `json:"version_iri"`
		Label      domain.Translations `json:"ontology_label"`
		Active     *bool               `json:"is_active"`
		Classes    int64               `json:"class_count"`
		Properties int64               `json:"property_count"`
	}
}

func ontologyRows(v projectsnapshot.CompleteProject, root projectsnapshot.Identity, links []projectsnapshot.OntologyLinkRecord) ([]OntologyRow, error) {
	evidence, err := v.Evidence(root)
	if err != nil {
		return nil, err
	}
	bodies, err := ontologyBodies(evidence)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(links, func(a, b projectsnapshot.OntologyLinkRecord) int {
		return strings.Compare(a.OntologyVersionID, b.OntologyVersionID)
	})
	rows := make([]OntologyRow, 0, len(links))
	for _, link := range links {
		raw, ok := bodies[link.OntologyVersionID]
		if !ok {
			return nil, fmt.Errorf("CSV ontology %s is missing from retained evidence", link.OntologyVersionID)
		}
		var body csvOntology
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			return nil, fmt.Errorf("decode CSV ontology: %w", err)
		}
		if body.Version.ID != link.OntologyVersionID {
			return nil, fmt.Errorf("CSV ontology identity differs from link")
		}
		rows = append(rows, OntologyRow{OntologyName: body.Ontology.Name, OntologyLabel: body.Version.Label.Get("en"), Version: body.Version.Version, OntologyURI: body.Version.URI, VersionIRI: body.Version.IRI, IsPrimary: value(link.IsPrimary), IsActive: body.Version.Active, ClassCount: body.Version.Classes, PropertyCount: body.Version.Properties, AddedAt: string(link.AddedAt), UsageNotes: value(link.UsageNotes)})
	}
	return rows, nil
}

func ontologyBodies(e projectsnapshot.Evidence) (map[string]string, error) {
	bodies := map[string]string{}
	if e.Contract() == 2 {
		a, err := e.Legacy()
		if err != nil {
			return nil, err
		}
		doc, err := a.Decode()
		if err != nil {
			return nil, err
		}
		for _, b := range doc.Inventory {
			bodies[b.VersionID] = b.BodyJSON
		}
	} else {
		publications, err := e.Publications()
		if err != nil {
			return nil, err
		}
		for _, p := range publications {
			d, err := p.Artifact.Decode()
			if err != nil {
				return nil, err
			}
			bodies[d.VersionID] = d.BodyJSON
		}
	}
	return bodies, nil
}
