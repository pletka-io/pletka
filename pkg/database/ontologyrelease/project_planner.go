package ontologyrelease

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/projectontology"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/ontologyartifact"
)

type projectPlanner struct{ queries *sqlcgen.Queries }

func (r projectPlanner) Candidates(ctx context.Context) ([]projectontology.Candidate, error) {
	rows, err := r.queries.ProjectOntologyCandidates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]projectontology.Candidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectontology.Candidate{OntologyID: row.OntologyID, Prefix: row.Prefix, VersionID: row.VersionID, Active: row.Active, Published: row.Published})
	}
	return out, nil
}
func (r projectPlanner) Publication(ctx context.Context, version string) (ontologyartifact.Artifact, error) {
	row, err := r.queries.ProjectOntologyPublication(ctx, version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ontologyartifact.Artifact{}, &projectontology.InvalidError{Detail: fmt.Sprintf("ontology version %s is not published", version)}
	}
	if err != nil {
		return ontologyartifact.Artifact{}, err
	}
	return ontologyartifact.Artifact{JSON: row.Artifact, Digest: row.Digest}, nil
}
