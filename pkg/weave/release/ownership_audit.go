package release

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/database/txscope"
)

// OwnershipAuditEntry identifies history requiring evidence before it can be verified.
type OwnershipAuditEntry struct {
	Issue                      string    `json:"issue"`
	ProjectID                  string    `json:"project_id"`
	Version                    string    `json:"version"`
	CreatedAt                  time.Time `json:"created_at"`
	SameVersionProjects        int64     `json:"same_version_projects"`
	PossibleParentKeyCollision bool      `json:"possible_parent_key_collision"`
}

// AuditArchiveOwnership reports unverified releases without changing their data.
func AuditArchiveOwnership(ctx context.Context, pool *pgxpool.Pool) ([]OwnershipAuditEntry, error) {
	rows, err := sqlcgen.New(txscope.New(pool)).WeaveAuditReleaseArchiveOwnership(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit release archive ownership: %w", err)
	}
	out := make([]OwnershipAuditEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, OwnershipAuditEntry{Issue: row.Issue, ProjectID: row.ProjectID, Version: row.Version, CreatedAt: row.CreatedAt, SameVersionProjects: row.SameVersionProjects, PossibleParentKeyCollision: row.PossibleParentKeyCollision})
	}
	return out, nil
}
