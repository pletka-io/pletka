package readsession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	readsql "github.com/pletka-io/pletka/pkg/database/readsql/projectontologies"
	"github.com/pletka-io/pletka/pkg/ontologyartifact"
)

// OntologyReader reads only the verified ontology inventory of its bound project
// release. Values may escape; the reader shares its session's transaction and
// lifetime. It cannot select another project, version, or ontology policy.
type OntologyReader interface {
	Artifacts(context.Context) ([]ontologyartifact.ProjectArtifact, error)
	References(context.Context) ([]ontologyartifact.TermResolution, error)
}

type ontologyReader struct {
	queries  readsql.Querier
	project  string
	version  string
	lifetime *lifetime
}

// Ontologies binds the verified publication facet on the existing transaction.
// Draft, closed, zero and legacy-only sessions cannot acquire this capability;
// preserved legacy evidence remains the separate LegacyOntology value contract.
func (s *Session) Ontologies(ctx context.Context) (OntologyReader, error) {
	if s == nil || s.lifetime == nil || s.lifetime.closed || !s.scope.IsRelease() || s.ontologies == nil {
		return nil, fmt.Errorf("ontology reader requires an open release session")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	verified, err := s.ontologies.ProjectOntologyInventory(ctx, readsql.ProjectOntologyInventoryParams{ProjectID: s.projectID, Version: s.scope.Version()})
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !verified {
		return nil, &NotInReleaseError{ProjectID: s.projectID, Version: s.scope.Version(), EntityID: "verified ontology inventory"}
	}
	if err != nil {
		return nil, err
	}
	return &ontologyReader{queries: s.ontologies, project: s.projectID, version: s.scope.Version(), lifetime: s.lifetime}, nil
}

func (r *ontologyReader) check(ctx context.Context) error {
	if r.lifetime == nil || r.lifetime.closed {
		return fmt.Errorf("project ontology reader is closed")
	}
	return ctx.Err()
}

func (r *ontologyReader) References(ctx context.Context) ([]ontologyartifact.TermResolution, error) {
	if err := r.check(ctx); err != nil {
		return nil, err
	}
	data, err := r.queries.ProjectOntologyReferences(ctx, readsql.ProjectOntologyReferencesParams{ProjectID: r.project, Version: r.version})
	if err != nil {
		return nil, err
	}
	var refs []ontologyartifact.TermResolution
	if err := json.Unmarshal(data, &refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func (r *ontologyReader) Artifacts(ctx context.Context) ([]ontologyartifact.ProjectArtifact, error) {
	if err := r.check(ctx); err != nil {
		return nil, err
	}
	rows, err := r.queries.ProjectOntologyArtifacts(ctx, readsql.ProjectOntologyArtifactsParams{ProjectID: r.project, Version: r.version})
	if err != nil {
		return nil, err
	}
	out := make([]ontologyartifact.ProjectArtifact, 0, len(rows))
	for _, row := range rows {
		a := ontologyartifact.Artifact{JSON: row.Artifact, Digest: row.Digest}
		doc, err := a.Decode()
		if err != nil {
			return nil, err
		}
		if doc.OntologyID != row.OntologyID || doc.VersionID != row.OntologyVersionID {
			return nil, fmt.Errorf("project ontology pin ownership mismatch")
		}
		out = append(out, ontologyartifact.ProjectArtifact{Artifact: a, Linked: row.Linked, Primary: row.IsPrimary})
	}
	return out, nil
}
