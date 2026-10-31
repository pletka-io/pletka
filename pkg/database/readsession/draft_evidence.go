package readsession

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/projectontology"
	"github.com/pletka-io/pletka/pkg/database/readsql/draft"
	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

// DraftActors loads only actors referenced by this working project's content.
func (s *Session) DraftActors(ctx context.Context) ([]releasedescriptor.Actor, error) {
	q, err := s.DraftQueries()
	if err != nil {
		return nil, err
	}
	rows, err := q.DraftActorDescriptors(ctx, s.projectID)
	if err != nil {
		return nil, err
	}
	out := make([]releasedescriptor.Actor, 0, len(rows))
	for _, row := range rows {
		if !row.Present {
			return nil, fmt.Errorf("required draft actor %s is absent", row.ActorID)
		}
		out = append(out, releasedescriptor.Actor{ActorID: row.ActorID, Type: row.Type, Slug: row.Slug, DisplayName: row.DisplayName})
	}
	return out, nil
}

// DraftPublications plans linked/explicit/defaults publications on this snapshot,
// using the same selection algorithm as capture. Policy is determined from the
// whole project, even when a caller assembles only one entity.
// Source pins are loaded through this session's authorized dependency graph.
func (s *Session) DraftPublications(ctx context.Context) ([]ontologyartifact.Artifact, error) {
	q, err := s.DraftQueries()
	if err != nil {
		return nil, err
	}
	selected, err := q.DraftOntologySelections(ctx, s.projectID)
	if err != nil {
		return nil, err
	}
	sources, err := s.Dependencies(ctx)
	if err != nil {
		return nil, err
	}
	pins := []ontologyartifact.Pin{}
	for _, source := range sources {
		bound, err := s.ProjectDependency(ctx, source.ProjectID)
		if err != nil {
			return nil, err
		}
		reader, err := bound.Ontologies(ctx)
		if err != nil {
			return nil, err
		}
		artifacts, err := reader.Artifacts(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range artifacts {
			d, err := a.Artifact.Decode()
			if err != nil {
				return nil, err
			}
			pins = append(pins, ontologyartifact.Pin{OntologyID: d.OntologyID, VersionID: d.VersionID, Digest: a.Artifact.Digest})
		}
	}
	hasReferences, err := q.DraftHasSemanticReferences(ctx, s.projectID)
	if err != nil {
		return nil, err
	}
	return projectontology.Plan(ctx, draftOntologyPlanner{queries: q}, selected, pins, hasReferences)
}

type draftOntologyPlanner struct{ queries draft.Querier }

func (r draftOntologyPlanner) Candidates(ctx context.Context) ([]projectontology.Candidate, error) {
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
func (r draftOntologyPlanner) Publication(ctx context.Context, version string) (ontologyartifact.Artifact, error) {
	row, err := r.queries.ProjectOntologyPublication(ctx, version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ontologyartifact.Artifact{}, &projectontology.InvalidError{Detail: fmt.Sprintf("ontology version %s is not published", version)}
	}
	if err != nil {
		return ontologyartifact.Artifact{}, err
	}
	return ontologyartifact.Artifact{JSON: row.Artifact, Digest: row.Digest}, nil
}
