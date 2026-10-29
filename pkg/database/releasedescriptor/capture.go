package releasedescriptor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// Capture runs inside the release transaction, after archives and dependency pins
// exist and before archive ownership is finalized. It never backfills old cuts.
func Capture(ctx context.Context, q *sqlcgen.Queries, project, version string, hosts Hosts) error {
	modules, err := publishedModules(ctx, q, project, version, hosts)
	if err != nil {
		return err
	}
	return capture(ctx, q, project, version, hosts, modules)
}

// CaptureLegacy captures descriptors from sealed snapshot bodies. It cannot
// consult mutable ontology catalog data or imply verified publication evidence.
func CaptureLegacy(ctx context.Context, q *sqlcgen.Queries, project, version string, hosts Hosts, a ontologysnapshot.Artifact) error {
	doc, err := a.Decode()
	if err != nil {
		return err
	}
	if doc.ProjectID != project || doc.Version != version {
		return &InvalidError{Message: "legacy descriptor source identity mismatch"}
	}
	modules := map[string]string{}
	for _, b := range doc.Inventory {
		var body struct {
			Ontology struct{ ID, Prefix, Name string }
		}
		if err := json.Unmarshal([]byte(b.BodyJSON), &body); err != nil {
			return err
		}
		modules[b.VersionID] = ontologyModule(hosts, body.Ontology.Prefix, body.Ontology.Name, body.Ontology.ID)
	}
	return capture(ctx, q, project, version, hosts, modules)
}

func capture(ctx context.Context, q *sqlcgen.Queries, project, version string, hosts Hosts, modules map[string]string) error {
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
	for id, module := range modules {
		if _, ok := d.OntologyModules[id]; !ok {
			d.OntologyModules[id] = module
		}
	}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return q.WeaveInsertReleaseDescriptors(ctx, sqlcgen.WeaveInsertReleaseDescriptorsParams{ProjectID: project, Version: version, Document: data})
}

func ontologyModule(hosts Hosts, prefix, name, id string) string {
	if hosts.Ontology == "" {
		hosts.Ontology = DefaultOntologyHost
	}
	return hosts.Ontology + "/" + OntologySlug(prefix, name, id)
}

func publishedModules(ctx context.Context, q *sqlcgen.Queries, project, version string, hosts Hosts) (map[string]string, error) {
	modules := map[string]string{}
	ontologies, err := q.WeaveReleaseDescriptorOntologies(ctx, sqlcgen.WeaveReleaseDescriptorOntologiesParams{ProjectID: project, Version: version})
	if err != nil {
		return nil, err
	}
	if hosts.Ontology == "" {
		hosts.Ontology = DefaultOntologyHost
	}
	for _, o := range ontologies {
		content, err := (ontologyrelease.Artifact{JSON: o.Artifact, Digest: o.Digest}).Content()
		if err != nil {
			return nil, err
		}
		if content.Ontology.ID != o.OntologyID || content.Version.ID != o.OntologyVersionID {
			return nil, fmt.Errorf("ontology module pin ownership mismatch")
		}
		modules[o.OntologyVersionID] = ontologyModule(hosts, content.Ontology.Prefix, content.Ontology.Name, content.Ontology.ID)
	}
	return modules, nil
}
