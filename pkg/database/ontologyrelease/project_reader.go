package ontologyrelease

import (
	"context"
	"encoding/json"
	"fmt"

	readsql "github.com/pletka-io/pletka/pkg/database/readsql/projectontologies"
)

// ProjectReader can only read the ontology inventory of its bound project cut.
type ProjectReader interface {
	Artifacts(context.Context) ([]ProjectArtifact, error)
	References(context.Context) ([]TermResolution, error)
}

type projectReader struct {
	queries readsql.Querier
	project string
	version string
	closed  bool
}

func (r *projectReader) References(ctx context.Context) ([]TermResolution, error) {
	if r.closed {
		return nil, fmt.Errorf("project ontology reader is closed")
	}
	data, err := r.queries.ProjectOntologyReferences(ctx, readsql.ProjectOntologyReferencesParams{ProjectID: r.project, Version: r.version})
	if err != nil {
		return nil, err
	}
	var refs []TermResolution
	if err := json.Unmarshal(data, &refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func (r *projectReader) Artifacts(ctx context.Context) ([]ProjectArtifact, error) {
	if r.closed {
		return nil, fmt.Errorf("project ontology reader is closed")
	}
	rows, err := r.queries.ProjectOntologyArtifacts(ctx, readsql.ProjectOntologyArtifactsParams{ProjectID: r.project, Version: r.version})
	if err != nil {
		return nil, err
	}
	out := make([]ProjectArtifact, 0, len(rows))
	for _, row := range rows {
		a := Artifact{JSON: row.Artifact, Digest: row.Digest}
		doc, err := a.Decode()
		if err != nil {
			return nil, err
		}
		if doc.OntologyID != row.OntologyID || doc.VersionID != row.OntologyVersionID {
			return nil, fmt.Errorf("project ontology pin ownership mismatch")
		}
		out = append(out, ProjectArtifact{Artifact: a, Linked: row.Linked, Primary: row.IsPrimary})
	}
	return out, nil
}
