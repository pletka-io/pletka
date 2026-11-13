package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// GraftOverrideStore reads weave_graft_overrides for the generator service.
type GraftOverrideStore struct{ q *sqlcgen.Queries }

func NewGraftOverrideStore(pool *pgxpool.Pool) GraftOverrideStore {
	return GraftOverrideStore{q: sqlcgen.New(pool)}
}

// GraftOverrides returns the model's overrides keyed by address.
func (s GraftOverrideStore) GraftOverrides(ctx context.Context, projectID, modelID string) (map[string]domain.GraftOverride, error) {
	rows, err := s.q.WeaveListGraftOverridesByModel(ctx, sqlcgen.WeaveListGraftOverridesByModelParams{ProjectID: projectID, ModelID: modelID})
	if err != nil {
		return nil, err
	}
	out := make(map[string]domain.GraftOverride, len(rows))
	for _, r := range rows {
		g := domain.GraftOverride{Address: r.Address, IsHidden: r.IsHidden, MaxOccursSet: r.MaxOccursSet}
		if r.MaxOccurs != nil {
			n := int(*r.MaxOccurs)
			g.MaxOccurs = &n
		}
		if len(r.DisplayName) > 0 {
			if err := json.Unmarshal(r.DisplayName, &g.DisplayName); err != nil {
				return nil, fmt.Errorf("graft %s display_name: %w", r.Address, err)
			}
		}
		out[r.Address] = g
	}
	return out, nil
}
