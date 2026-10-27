package gitmaterializer

import (
	"context"
	"errors"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/ontologybody"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
)

const ontologyArtifactFile = "artifact.json"

type ontologyVendorArtifact struct {
	File   string `json:"file"`
	Digest string `json:"digest"`
}

func (m *Materializer) projectOntologyArtifacts(ctx context.Context, project, version string) ([]ontologyrelease.ProjectArtifact, error) {
	var values []ontologyrelease.ProjectArtifact
	err := ontologyrelease.NewFactory(m.pool).ReadProject(ctx, project, version, func(r ontologyrelease.ProjectReader) error { var err error; values, err = r.Artifacts(ctx); return err })
	return values, err
}

// This optional lookup belongs only to draft/legacy materialization. Released
// consumers enter through ReadProject and never take this fallback.
func (m *Materializer) publishedOntologyClosure(ctx context.Context, owner, version string) ([]ontologyrelease.Artifact, error) {
	ref, err := ontologyrelease.NewReference(ontologybody.OntologyID(owner), ontologybody.VersionID(version))
	if err != nil {
		return nil, err
	}
	var values []ontologyrelease.Artifact
	entered := false
	err = ontologyrelease.NewFactory(m.pool).Read(ctx, ref, func(r ontologyrelease.Reader) error {
		entered = true
		root, err := r.Artifact(ctx)
		if err != nil {
			return err
		}
		doc, err := root.Decode()
		if err != nil {
			return err
		}
		values = append(values, root)
		for _, pin := range doc.Pins {
			a, err := r.Dependency(ctx, ontologybody.OntologyID(pin.OntologyID))
			if err != nil {
				return err
			}
			values = append(values, a)
		}
		return nil
	})
	var missing interface{ IsNotInRelease() bool }
	if !entered && errors.As(err, &missing) && missing.IsNotInRelease() {
		return nil, nil
	}
	return values, err
}

func (m *Materializer) artifactRequirement(a ontologyrelease.Artifact) (pletkaOntologyRequirement, error) {
	c, err := a.Content()
	if err != nil {
		return pletkaOntologyRequirement{}, err
	}
	return pletkaOntologyRequirement{Module: m.ontologyModulePath(ontologyModuleSlug(c.Ontology.Prefix, c.Ontology.Name, c.Ontology.ID)), OntologyID: c.Ontology.ID, OntologyVersionID: c.Version.ID, Version: c.Version.Version, Digest: a.Digest, Artifact: &a}, nil
}
func uniqueOntologyRequirements(in []pletkaOntologyRequirement) ([]pletkaOntologyRequirement, error) {
	byOwner := map[string]pletkaOntologyRequirement{}
	out := make([]pletkaOntologyRequirement, 0, len(in))
	for _, r := range in {
		if old, ok := byOwner[r.OntologyID]; ok {
			if old.Artifact == nil && r.Artifact == nil && old.OntologyVersionID != r.OntologyVersionID {
				out = append(out, r)
				continue
			}
			if old.OntologyVersionID != r.OntologyVersionID || old.Digest != r.Digest {
				return nil, fmt.Errorf("conflicting ontology requirements for %s", r.OntologyID)
			}
			continue
		}
		byOwner[r.OntologyID] = r
		out = append(out, r)
	}
	return out, nil
}
