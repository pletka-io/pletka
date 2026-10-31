// Package projectrestore imports complete historical graphs and working copies.
// It owns one transaction and root-first restore locks; it never recaptures input
// from hot tables, chooses source versions or changes ontology default policy.
package projectrestore

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/releasecontent"
	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

type dependency struct {
	ProjectID     string `json:"project_id"`
	Version       string `json:"version_number"`
	SourceProject string `json:"source_project_id"`
	SourceVersion string `json:"source_version"`
}
type member struct {
	dependency
	Kind string `json:"entity_type"`
	ID   string `json:"entity_id"`
}
type publication struct {
	ProjectID  string `json:"project_id"`
	Version    string `json:"version_number"`
	OntologyID string `json:"ontology_id"`
	VersionID  string `json:"ontology_version_id"`
	Digest     string `json:"digest"`
}
type receipt struct {
	Format   int                               `json:"format_version"`
	Sections map[string]releasecontent.Section `json:"sections"`
	Kind     string                            `json:"evidence_kind,omitempty"`
	Digest   string                            `json:"ontology_snapshot_digest,omitempty"`
}
type payload struct {
	Format        int                                     `json:"format_version"`
	Header        projectbundle.ReleaseRecord             `json:"header"`
	Sections      map[string][]map[string]json.RawMessage `json:"sections"`
	Receipt       *receipt                                `json:"receipt"`
	Descriptors   releasedescriptor.Document              `json:"descriptors"`
	Dependencies  []dependency                            `json:"dependencies"`
	Membership    []member                                `json:"membership"`
	Publications  []publication                           `json:"publications"`
	References    []ontologyartifact.TermResolution       `json:"references"`
	Legacy        *ontologyevidence.Artifact              `json:"legacy"`
	LegacyContext string                                  `json:"legacy_context"`
}
type project struct {
	id       projectsnapshot.Identity
	document []byte
}
type prepared struct {
	root         string
	projects     []project
	publications []ontologyartifact.Artifact
	legacy       []ontologyevidence.Artifact
}

func prepare(imported projectbundle.ImportedProject, catalog projectbundle.Catalog) (prepared, error) {
	value := imported.Snapshot()
	root, err := value.Root()
	if err != nil {
		return prepared{}, err
	}
	records, err := catalog.SelectedReleases(value)
	if err != nil {
		return prepared{}, err
	}
	headers := map[projectsnapshot.Identity]projectbundle.ReleaseRecord{}
	for _, r := range records {
		id, err := projectsnapshot.Release(r.ProjectID, r.Version)
		if err != nil {
			return prepared{}, err
		}
		headers[id] = r
	}
	ids, err := value.Projects()
	if err != nil {
		return prepared{}, err
	}
	inputs := map[projectsnapshot.Identity]projectsnapshot.ProjectInput{}
	p := prepared{root: root.ProjectID()}
	contexts := map[projectsnapshot.Identity]string{}
	for _, id := range ids {
		in, err := value.Input(id)
		if err != nil {
			return p, err
		}
		inputs[id] = in
		if err := p.interpretation(in, contexts); err != nil {
			return p, err
		}
	}
	visited := map[projectsnapshot.Identity]bool{}
	var visit func(projectsnapshot.Identity) error
	visit = func(id projectsnapshot.Identity) error {
		if visited[id] {
			return nil
		}
		visited[id] = true
		in := inputs[id]
		for _, dep := range in.Dependencies {
			if err := visit(dep); err != nil {
				return err
			}
		}
		header := headers[id]
		if id.IsDraft() {
			header = projectbundle.ReleaseRecord{ProjectID: id.ProjectID()}
		}
		data, err := encodePayload(in, header, contexts[id])
		if err != nil {
			return err
		}
		p.projects = append(p.projects, project{id: id, document: data})
		return nil
	}
	if err := visit(root); err != nil {
		return p, err
	}
	return p, nil
}

func encodePayload(in projectsnapshot.ProjectInput, header projectbundle.ReleaseRecord, legacyContext string) ([]byte, error) {
	id := in.Evidence.Identity()
	sections, err := storageSections(in.Content)
	if err != nil {
		return nil, err
	}
	p := payload{Format: 1, Header: header, Sections: sections, Descriptors: in.Evidence.Descriptors().Document(), Dependencies: []dependency{}, Membership: []member{}, Publications: []publication{}, References: []ontologyartifact.TermResolution{}, LegacyContext: legacyContext}
	for _, dep := range in.Dependencies {
		p.Dependencies = append(p.Dependencies, dependency{ProjectID: id.ProjectID(), Version: id.Version(), SourceProject: dep.ProjectID(), SourceVersion: dep.Version()})
	}
	for _, m := range in.Membership {
		p.Membership = append(p.Membership, member{dependency: dependency{ProjectID: id.ProjectID(), Version: id.Version(), SourceProject: m.Source.ProjectID(), SourceVersion: m.Source.Version()}, Kind: m.Kind, ID: m.ID})
	}
	if !id.IsDraft() {
		captured, err := in.Evidence.CaptureSections()
		if err != nil {
			return nil, err
		}
		p.Receipt = &receipt{Format: in.Evidence.Contract(), Sections: captured}
	}
	if in.Evidence.Contract() == 2 {
		a, err := in.Evidence.Legacy()
		if err != nil {
			return nil, err
		}
		p.Legacy = &a
		p.Receipt.Kind, p.Receipt.Digest = "legacy_ontology_snapshot", a.Digest
	} else {
		artifacts, err := in.Evidence.Publications()
		if err != nil {
			return nil, err
		}
		for _, a := range artifacts {
			doc, err := a.Artifact.Decode()
			if err != nil {
				return nil, err
			}
			p.Publications = append(p.Publications, publication{ProjectID: id.ProjectID(), Version: id.Version(), OntologyID: doc.OntologyID, VersionID: doc.VersionID, Digest: a.Artifact.Digest})
		}
		slices.SortFunc(p.Publications, func(a, b publication) int { return strings.Compare(a.OntologyID, b.OntologyID) })
		p.References, err = in.Evidence.References()
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(p)
}

// Convert the private byte freeze to nullable JSON text only at the database
// write boundary. Exact decimal/date/timestamp text is left for PostgreSQL's
// typed conversion; receipt hashes then independently check the resulting rows.
func storageSections(content projectsnapshot.Content) (map[string][]map[string]json.RawMessage, error) {
	raw, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	var sections map[string][]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		return nil, err
	}
	for _, section := range projectsnapshot.Coverage() {
		for _, row := range sections[section.Name] {
			for _, column := range section.JSONColumns {
				var raw []byte
				if err := json.Unmarshal(row[column], &raw); err != nil {
					return nil, err
				}
				if raw != nil {
					text, err := json.Marshal(string(raw))
					if err != nil {
						return nil, err
					}
					row[column] = text
				}
			}
		}
	}
	return sections, nil
}

func (p *prepared) interpretation(in projectsnapshot.ProjectInput, contexts map[projectsnapshot.Identity]string) error {
	if in.Evidence.Contract() == 2 {
		a, err := in.Evidence.Legacy()
		if err != nil {
			return err
		}
		p.legacy = append(p.legacy, a)
		doc, err := a.Decode()
		if err != nil {
			return err
		}
		for _, source := range doc.Sources {
			var local ontologyevidence.Document
			if err := json.Unmarshal([]byte(source.JSON), &local); err != nil {
				return err
			}
			id, err := projectsnapshot.Release(local.ProjectID, local.Version)
			if err != nil {
				return err
			}
			if old, ok := contexts[id]; ok && old != source.JSON {
				return fmt.Errorf("conflicting exact legacy source contexts")
			}
			contexts[id] = source.JSON
		}
	} else {
		publications, err := in.Evidence.Publications()
		if err != nil {
			return err
		}
		for _, a := range publications {
			p.publications = append(p.publications, a.Artifact)
		}
	}
	return nil
}
