package actorlabels

import (
	"context"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Reader interface {
	LabelsByID(ctx context.Context, ids []string) map[string]string
}

type PostgresReader struct {
	queries *sqlcgen.Queries
}

func NewPostgresReader(pool *pgxpool.Pool) *PostgresReader {
	return &PostgresReader{queries: sqlcgen.New(pool)}
}

func (r *PostgresReader) LabelsByID(ctx context.Context, ids []string) map[string]string {
	out := make(map[string]string, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		row, err := r.queries.WeaveGetActorByID(ctx, id)
		if err != nil {
			continue
		}
		if row.DisplayName != "" {
			out[id] = row.DisplayName
		}
	}
	return out
}
