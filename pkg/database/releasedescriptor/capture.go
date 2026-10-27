package releasedescriptor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// Capture runs inside the release transaction, after archives and dependency pins
// exist and before archive ownership is finalized. It never backfills old cuts.
func Capture(ctx context.Context, q *sqlcgen.Queries, project, version string, hosts Hosts) error {
	d := document{Format: 1, ProjectID: project, Version: version, Actors: map[string]Actor{}, ProjectModules: map[string]string{}, OntologyModules: map[string]string{}}
	actors, err := q.WeaveReleaseDescriptorActors(ctx, sqlcgen.WeaveReleaseDescriptorActorsParams{ProjectID: project, Version: version})
	if err != nil {
		return err
	}
	for _, a := range actors {
		if !a.Present {
			return &InvalidError{Message: fmt.Sprintf("required release actor %s is missing", a.ActorID)}
		}
		d.Actors[a.ActorID] = Actor{ActorID: a.ActorID, Type: a.Type, Slug: a.Slug, DisplayName: a.DisplayName}
	}
	owner, err := q.WeaveReleaseDescriptorOwner(ctx, sqlcgen.WeaveReleaseDescriptorOwnerParams{ProjectID: project, Version: version})
	if err != nil {
		return err
	}
	a, ok := d.Actors[owner]
	if !ok {
		return &InvalidError{Message: fmt.Sprintf("release owner %s is missing", owner)}
	}
	d.ProjectModules[project] = ProjectModule(hosts.Project, a, project)
	sources, err := q.WeaveReleaseDescriptorSources(ctx, sqlcgen.WeaveReleaseDescriptorSourcesParams{ProjectID: project, Version: version})
	if err != nil {
		return err
	}
	for _, source := range sources {
		s, err := Decode(source.Document, source.SourceProjectID, source.SourceVersion)
		if err != nil {
			return err
		}
		d.ProjectModules[source.SourceProjectID] = s.Module()
		// A shared ontology must keep the same module identity in every vendored
		// source. Conflicting historical identities require an explicit decision.
		for id, module := range s.doc.OntologyModules {
			if old, ok := d.OntologyModules[id]; ok && old != module {
				return &InvalidError{Message: fmt.Sprintf("conflicting captured ontology module for %s", id)}
			}
			d.OntologyModules[id] = module
		}
	}
	ontologies, err := q.WeaveReleaseDescriptorOntologies(ctx, sqlcgen.WeaveReleaseDescriptorOntologiesParams{ProjectID: project, Version: version})
	if err != nil {
		return err
	}
	if hosts.Ontology == "" {
		hosts.Ontology = DefaultOntologyHost
	}
	for _, o := range ontologies {
		content, err := (ontologyrelease.Artifact{JSON: o.Artifact, Digest: o.Digest}).Content()
		if err != nil {
			return err
		}
		if content.Ontology.ID != o.OntologyID || content.Version.ID != o.OntologyVersionID {
			return fmt.Errorf("ontology module pin ownership mismatch")
		}
		if _, ok := d.OntologyModules[o.OntologyVersionID]; !ok {
			d.OntologyModules[o.OntologyVersionID] = hosts.Ontology + "/" + OntologySlug(content.Ontology.Prefix, content.Ontology.Name, content.Ontology.ID)
		}
	}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return q.WeaveInsertReleaseDescriptors(ctx, sqlcgen.WeaveInsertReleaseDescriptorsParams{ProjectID: project, Version: version, Document: data})
}
