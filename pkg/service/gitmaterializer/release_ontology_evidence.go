package gitmaterializer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/database/releasedescriptor"
)

// Transitional assembly adapter. The captured contract chooses evidence once;
// serialization receives values and cannot request a hot ontology fallback.
type releaseOntologyEvidence struct {
	linked       []projectManifestLinkedOntology
	requirements []pletkaOntologyRequirement
	legacy       *ontologysnapshot.Artifact
}

func (m *Materializer) releaseOntologies(ctx context.Context, project, version string) (releaseOntologyEvidence, error) {
	out := releaseOntologyEvidence{requirements: []pletkaOntologyRequirement{}}
	var descriptors releasedescriptor.Snapshot
	var contract int16
	err := readsession.NewFactory(m.pool).Read(ctx, auth.Release(version), project, func(s *readsession.Session) error {
		var err error
		contract, err = s.ContentContract(ctx)
		if err != nil {
			return err
		}
		descriptors, err = s.Descriptors(ctx)
		if err != nil {
			return err
		}
		switch contract {
		case 0, 1:
			// Historical verified ontology inventories retain their original gate;
			// this does not certify complete structural capture for old cuts.
			return nil
		case 2:
			a, err := s.LegacyOntology(ctx)
			if err != nil {
				return err
			}
			out.legacy = &a
			return nil
		default:
			return fmt.Errorf("unsupported release content contract %d", contract)
		}
	})
	if err != nil {
		return out, err
	}
	if out.legacy != nil {
		return legacyOntologyValues(*out.legacy, descriptors)
	}
	values, err := m.projectOntologyArtifacts(ctx, project, version)
	if err != nil {
		return out, err
	}
	for _, v := range values {
		c, err := v.Artifact.Content()
		if err != nil {
			return out, err
		}
		module, err := descriptors.OntologyModule(c.Version.ID)
		if err != nil {
			return out, err
		}
		req, err := ontologyRequirement(v.Artifact, module)
		if err != nil {
			return out, err
		}
		out.requirements = append(out.requirements, req)
		if v.Linked {
			out.linked = append(out.linked, projectManifestLinkedOntology{OntologyID: c.Ontology.ID, OntologyVersionID: c.Version.ID, Version: c.Version.Version, IsPrimary: v.Primary})
		}
	}
	return out, nil
}

func legacyOntologyValues(a ontologysnapshot.Artifact, descriptors releasedescriptor.Snapshot) (releaseOntologyEvidence, error) {
	out := releaseOntologyEvidence{legacy: &a}
	d, err := a.Decode()
	if err != nil {
		return out, err
	}
	for _, b := range d.Inventory {
		var body struct {
			Version struct {
				Version string `json:"version_string"`
			}
		}
		var selection struct {
			Primary bool `json:"is_primary"`
		}
		if err := json.Unmarshal([]byte(b.BodyJSON), &body); err != nil {
			return out, err
		}
		if err := json.Unmarshal(b.Selection, &selection); err != nil {
			return out, err
		}
		out.linked = append(out.linked, projectManifestLinkedOntology{OntologyID: b.OntologyID, OntologyVersionID: b.VersionID, Version: body.Version.Version, IsPrimary: selection.Primary})
	}
	bodies, err := legacySnapshotBodies(a)
	if err != nil {
		return out, err
	}
	for _, b := range bodies {
		var body struct {
			Version struct {
				Version string `json:"version_string"`
			}
		}
		if err := json.Unmarshal([]byte(b.BodyJSON), &body); err != nil {
			return out, err
		}
		module, err := descriptors.OntologyModule(b.VersionID)
		if err != nil {
			return out, err
		}
		out.requirements = append(out.requirements, pletkaOntologyRequirement{Module: module, OntologyID: b.OntologyID, OntologyVersionID: b.VersionID, Version: body.Version.Version, BodyDigest: b.Digest})
	}
	sort.Slice(out.requirements, func(i, j int) bool {
		return out.requirements[i].OntologyVersionID < out.requirements[j].OntologyVersionID
	})
	return out, nil
}
