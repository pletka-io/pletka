package projectread

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/projectontology"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/namespace"
	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

func (a *Assembler) draftEvidence(ctx context.Context, s *readsession.Session, c projectsnapshot.Content, sources []projectsnapshot.ProjectInput) (projectsnapshot.Evidence, error) {
	elements, err := projectsnapshot.SemanticElements(c)
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	inputs, err := ontologyevidence.InputReferences(elements)
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	artifacts, err := s.DraftPublications(ctx)
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	linked := map[string]bool{}
	for _, link := range c.OntologyLinks {
		linked[link.OntologyVersionID] = link.IsPrimary != nil && *link.IsPrimary
	}
	publications := make([]ontologyartifact.ProjectArtifact, 0, len(artifacts))
	inventory := make([]ontologyevidence.Body, 0, len(artifacts))
	for _, artifact := range artifacts {
		d, err := artifact.Decode()
		if err != nil {
			return projectsnapshot.Evidence{}, err
		}
		primary, isLinked := linked[d.VersionID]
		publications = append(publications, ontologyartifact.ProjectArtifact{Artifact: artifact, Linked: isLinked, Primary: primary})
		// Body validation uses the sealed body digest; term receipts below retain
		// the publication digest that binds the full artifact and its dependencies.
		inventory = append(inventory, ontologyevidence.Body{OntologyID: d.OntologyID, VersionID: d.VersionID, BodyJSON: d.BodyJSON, Digest: d.BodyDigest})
	}
	refs, err := draftReferences(inputs, c.Namespaces, inventory, publications)
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	descriptors, err := a.draftDescriptors(ctx, s, c, sources, artifacts)
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	id, err := projectsnapshot.Draft(s.ProjectID())
	if err != nil {
		return projectsnapshot.Evidence{}, err
	}
	return projectsnapshot.DraftEvidence(id, descriptors, publications, refs)
}

func draftReferences(inputs []ontologyevidence.InputReference, aliases []projectsnapshot.NamespaceRecord, inventory []ontologyevidence.Body, publications []ontologyartifact.ProjectArtifact) ([]ontologyartifact.TermResolution, error) {
	bindings := make([]namespace.Binding, 0, len(aliases))
	for _, row := range aliases {
		bindings = append(bindings, namespace.Binding{Prefix: row.Prefix, Namespace: row.Namespace, Weight: row.Weight, Source: row.Source})
	}
	assessor, err := ontologyevidence.NewAssessor(inventory, bindings)
	if err != nil {
		return nil, err
	}
	digests := map[string]string{}
	for i, body := range inventory {
		digests[body.VersionID] = publications[i].Artifact.Digest
	}
	refs := make([]ontologyartifact.TermResolution, 0, len(inputs))
	for _, input := range inputs {
		status, candidates, err := assessor.Assess(input.Reference)
		if err != nil {
			return nil, err
		}
		if status != ontologyevidence.Resolved {
			return nil, &projectontology.InvalidError{Detail: fmt.Sprintf("%s %s%s: semantic input is %s", input.EntityKind, input.EntityID, input.Location, status)}
		}
		candidate := candidates[0]
		refs = append(refs, ontologyartifact.TermResolution{EntityKind: input.EntityKind, EntityID: input.EntityID, Location: input.Location, Reference: ontologyartifact.TermReference(input.Reference), OntologyID: candidate.OntologyID, VersionID: candidate.VersionID, Digest: digests[candidate.VersionID], URI: candidate.URI})
	}
	return refs, nil
}

func (a *Assembler) draftDescriptors(ctx context.Context, s *readsession.Session, c projectsnapshot.Content, sources []projectsnapshot.ProjectInput, artifacts []ontologyartifact.Artifact) (releasedescriptor.Snapshot, error) {
	if len(c.Projects) != 1 {
		return releasedescriptor.Snapshot{}, fmt.Errorf("draft project metadata is missing")
	}
	actors, err := s.DraftActors(ctx)
	if err != nil {
		return releasedescriptor.Snapshot{}, err
	}
	d := releasedescriptor.Document{Format: 1, ProjectID: s.ProjectID(), Actors: map[string]releasedescriptor.Actor{}, ProjectModules: map[string]string{}, OntologyModules: map[string]string{}}
	for _, actor := range actors {
		d.Actors[actor.ActorID] = actor
	}
	owner, ok := d.Actors[c.Projects[0].OwnerID]
	if !ok {
		return releasedescriptor.Snapshot{}, fmt.Errorf("draft owner descriptor is missing")
	}
	d.ProjectModules[s.ProjectID()] = releasedescriptor.ProjectModule(a.options.Hosts.Project, owner, s.ProjectID())
	for _, source := range sources {
		d.ProjectModules[source.Evidence.Identity().ProjectID()] = source.Evidence.Descriptors().Module()
		for version, module := range source.Evidence.Descriptors().OntologyModules() {
			if old, ok := d.OntologyModules[version]; ok && old != module {
				return releasedescriptor.Snapshot{}, fmt.Errorf("conflicting source ontology module for %s", version)
			}
			d.OntologyModules[version] = module
		}
	}
	host := a.options.Hosts.Ontology
	if host == "" {
		host = releasedescriptor.DefaultOntologyHost
	}
	for _, artifact := range artifacts {
		doc, err := artifact.Decode()
		if err != nil {
			return releasedescriptor.Snapshot{}, err
		}
		if _, ok := d.OntologyModules[doc.VersionID]; ok {
			continue
		}
		var body struct {
			Ontology struct{ ID, Prefix, Name string }
		}
		if err := json.Unmarshal([]byte(doc.BodyJSON), &body); err != nil {
			return releasedescriptor.Snapshot{}, err
		}
		d.OntologyModules[doc.VersionID] = host + "/" + releasedescriptor.OntologySlug(body.Ontology.Prefix, body.Ontology.Name, body.Ontology.ID)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return releasedescriptor.Snapshot{}, err
	}
	return releasedescriptor.Decode(raw, s.ProjectID(), "")
}
