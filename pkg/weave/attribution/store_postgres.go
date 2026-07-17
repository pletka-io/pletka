package attribution

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// postgresStore is the pgx + sqlc implementation of Store, backed by
// the weave_project_attributions table (migration 056).
type postgresStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

var _ Store = (*postgresStore)(nil)

// NewPostgresStore returns a Store backed by the supplied pgx pool.
func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{
		queries: sqlcgen.New(pool),
		pool:    pool,
	}
}

func (s *postgresStore) ListForProject(ctx context.Context, projectID string) ([]domain.Attribution, error) {
	rows, err := s.queries.WeaveListProjectAttributions(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project attributions: %w", err)
	}
	out := make([]domain.Attribution, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Attribution{
			ProjectID: r.ProjectID,
			ActorID:   r.ActorID,
			Kind:      r.Kind,
			Position:  int(r.Position),
			Note:      r.Note,
			CreatedAt: r.CreatedAt,
			ActorName: r.ActorName,
			ActorType: r.ActorType,
		})
	}
	return out, nil
}

func (s *postgresStore) ListForProjects(ctx context.Context, projectIDs []string) (map[string][]domain.Attribution, error) {
	if len(projectIDs) == 0 {
		return map[string][]domain.Attribution{}, nil
	}
	rows, err := s.queries.WeaveListProjectAttributionsForProjects(ctx, projectIDs)
	if err != nil {
		return nil, fmt.Errorf("list project attributions batch: %w", err)
	}
	out := make(map[string][]domain.Attribution, len(projectIDs))
	for _, r := range rows {
		out[r.ProjectID] = append(out[r.ProjectID], domain.Attribution{
			ProjectID: r.ProjectID,
			ActorID:   r.ActorID,
			Kind:      r.Kind,
			Position:  int(r.Position),
			Note:      r.Note,
			CreatedAt: r.CreatedAt,
			ActorName: r.ActorName,
			ActorType: r.ActorType,
		})
	}
	return out, nil
}

func (s *postgresStore) Create(ctx context.Context, projectID string, in Input) (domain.Attribution, error) {
	row, err := s.queries.WeaveInsertProjectAttribution(ctx, sqlcgen.WeaveInsertProjectAttributionParams{
		ProjectID: projectID,
		ActorID:   in.ActorID,
		Kind:      in.Kind,
		Note:      in.Note,
	})
	if err != nil {
		return domain.Attribution{}, fmt.Errorf("insert project attribution: %w", err)
	}
	return domain.Attribution{
		ProjectID: row.ProjectID,
		ActorID:   row.ActorID,
		Kind:      row.Kind,
		Position:  int(row.Position),
		Note:      row.Note,
		CreatedAt: row.CreatedAt,
	}, nil
}

func (s *postgresStore) CreateAt(ctx context.Context, projectID, actorID, kind string, position int, note string) (domain.Attribution, error) {
	row, err := s.queries.WeaveInsertProjectAttributionAt(ctx, sqlcgen.WeaveInsertProjectAttributionAtParams{
		ProjectID: projectID,
		ActorID:   actorID,
		Kind:      kind,
		Position:  int32(position),
		Note:      note,
	})
	if err != nil {
		return domain.Attribution{}, fmt.Errorf("upsert project attribution: %w", err)
	}
	return domain.Attribution{
		ProjectID: row.ProjectID,
		ActorID:   row.ActorID,
		Kind:      row.Kind,
		Position:  int(row.Position),
		Note:      row.Note,
		CreatedAt: row.CreatedAt,
	}, nil
}

func (s *postgresStore) UpdateNote(ctx context.Context, projectID, actorID, kind string, position int, note string) error {
	if err := s.queries.WeaveUpdateProjectAttributionNote(ctx, sqlcgen.WeaveUpdateProjectAttributionNoteParams{
		ProjectID: projectID,
		ActorID:   actorID,
		Kind:      kind,
		Position:  int32(position),
		Note:      note,
	}); err != nil {
		return fmt.Errorf("update project attribution note: %w", err)
	}
	return nil
}

func (s *postgresStore) Delete(ctx context.Context, projectID, actorID, kind string, position int) error {
	if err := s.queries.WeaveDeleteProjectAttribution(ctx, sqlcgen.WeaveDeleteProjectAttributionParams{
		ProjectID: projectID,
		ActorID:   actorID,
		Kind:      kind,
		Position:  int32(position),
	}); err != nil {
		return fmt.Errorf("delete project attribution: %w", err)
	}
	return nil
}

func (s *postgresStore) DeleteByKind(ctx context.Context, projectID, kind string) error {
	if err := s.queries.WeaveDeleteProjectAttributionsByKind(ctx, sqlcgen.WeaveDeleteProjectAttributionsByKindParams{
		ProjectID: projectID,
		Kind:      kind,
	}); err != nil {
		return fmt.Errorf("delete project attributions by kind: %w", err)
	}
	return nil
}

func (s *postgresStore) SetPosition(ctx context.Context, projectID, actorID, kind string, oldPosition, newPosition int) error {
	if err := s.queries.WeaveSetProjectAttributionPosition(ctx, sqlcgen.WeaveSetProjectAttributionPositionParams{
		ProjectID:  projectID,
		ActorID:    actorID,
		Kind:       kind,
		Position:   int32(oldPosition),
		Position_2: int32(newPosition),
	}); err != nil {
		return fmt.Errorf("set project attribution position: %w", err)
	}
	return nil
}

func (s *postgresStore) ListAllActors(ctx context.Context) ([]ActorOption, error) {
	rows, err := s.queries.WeaveListActors(ctx)
	if err != nil {
		return nil, fmt.Errorf("list actors: %w", err)
	}
	out := make([]ActorOption, 0, len(rows))
	for _, r := range rows {
		out = append(out, ActorOption{
			ID:          r.ID,
			DisplayName: r.DisplayName,
			Type:        r.Type,
			Slug:        r.Slug,
		})
	}
	return out, nil
}
