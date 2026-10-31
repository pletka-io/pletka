package projectbundle

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Encode returns deterministic canonical files from a complete detached value.
// Both draft and release use this path. Row paths include the complete captured
// primary key; no generated entity IDs, foreign rebinding or live lookups occur.
func Encode(value projectsnapshot.CompleteProject) (Files, error) {
	root, err := value.Root()
	if err != nil {
		return nil, err
	}
	ids, err := value.Projects()
	if err != nil {
		return nil, err
	}
	files := Files{}
	index := manifest{SchemaVersion: SchemaVersion, Root: selected(root), Projects: []fileRef{}}
	for _, id := range ids {
		in, err := value.Input(id)
		if err != nil {
			return nil, err
		}
		ref, err := encodeContext(files, in)
		if err != nil {
			return nil, err
		}
		index.Projects = append(index.Projects, ref)
	}
	data, err := canonical(index)
	if err != nil {
		return nil, err
	}
	files[ManifestPath] = data
	return files, nil
}

func encodeContext(files Files, in projectsnapshot.ProjectInput) (fileRef, error) {
	id := selected(in.Evidence.Identity())
	context := contextFile{Identity: id, Dependencies: []selection{}, Membership: []membership{}, Sections: map[string][]fileRef{}}
	for _, dep := range in.Dependencies {
		context.Dependencies = append(context.Dependencies, selected(dep))
	}
	for _, member := range in.Membership {
		context.Membership = append(context.Membership, membership{Kind: member.Kind, ID: member.ID, Source: selected(member.Source)})
	}
	var err error
	context.Evidence, err = encodeEvidence(files, in.Evidence)
	if err != nil {
		return fileRef{}, err
	}
	data, err := json.Marshal(in.Content)
	if err != nil {
		return fileRef{}, err
	}
	var sections map[string][]json.RawMessage
	if err := json.Unmarshal(data, &sections); err != nil {
		return fileRef{}, err
	}
	for _, section := range projectsnapshot.Coverage() {
		refs := make([]fileRef, 0, len(sections[section.Name]))
		for _, row := range sections[section.Name] {
			data, key, err := portableRow(row, section)
			if err != nil {
				return fileRef{}, err
			}
			path := contextPath(id) + "/rows/" + section.Name + "/" + key + ".json"
			refs = append(refs, files.put(path, data))
		}
		slices.SortFunc(refs, func(a, b fileRef) int { return strings.Compare(a.Path, b.Path) })
		context.Sections[section.Name] = refs
	}
	data, err = canonical(context)
	if err != nil {
		return fileRef{}, err
	}
	return files.put(contextPath(id)+"/context.json", data), nil
}

func encodeEvidence(files Files, e projectsnapshot.Evidence) (evidenceFile, error) {
	out := evidenceFile{Contract: e.Contract(), Descriptors: e.Descriptors().Document()}
	if e.Contract() != 0 {
		sections, err := e.CaptureSections()
		if err != nil {
			return evidenceFile{}, err
		}
		out.Receipt = &receiptFile{Format: e.Contract(), Sections: sections}
	}
	if e.Contract() == 2 {
		a, err := e.Legacy()
		if err != nil {
			return evidenceFile{}, err
		}
		ref := files.put("ontologies/legacy/"+a.Digest+".json", []byte(a.JSON))
		out.Legacy = &ref
		out.Receipt.Kind, out.Receipt.Digest = "legacy_ontology_snapshot", a.Digest
		return out, nil
	}
	publications, err := e.Publications()
	if err != nil {
		return evidenceFile{}, err
	}
	out.Publications = make([]publication, 0, len(publications))
	for _, p := range publications {
		ref := files.put("ontologies/published/"+p.Artifact.Digest+".json", []byte(p.Artifact.JSON))
		out.Publications = append(out.Publications, publication{Artifact: ref, Linked: p.Linked, Primary: p.Primary})
	}
	slices.SortFunc(out.Publications, func(a, b publication) int { return strings.Compare(a.Artifact.Path, b.Artifact.Path) })
	out.References, err = e.References()
	if err != nil {
		return evidenceFile{}, err
	}
	slices.SortFunc(out.References, func(a, b ontologyartifact.TermResolution) int {
		if c := strings.Compare(a.EntityKind, b.EntityKind); c != 0 {
			return c
		}
		if c := strings.Compare(a.EntityID, b.EntityID); c != 0 {
			return c
		}
		return strings.Compare(a.Location, b.Location)
	})
	return out, nil
}
