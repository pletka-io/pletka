package release

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/database/txscope"
)

// DependencyAuditEntry identifies a release unavailable as a new dependency.
// It describes project inventory only, not ontology or output completeness.
type DependencyAuditEntry struct {
	ProjectID               string    `json:"project_id"`
	Version                 string    `json:"version"`
	CreatedAt               time.Time `json:"created_at"`
	ArchiveOwnershipVersion int16     `json:"archive_ownership_version"`
}

// AuditProjectDependencies reports releases needing historical evidence or a
// new cut before consumers may pin them. It never modifies or certifies history.
func AuditProjectDependencies(ctx context.Context, pool *pgxpool.Pool) ([]DependencyAuditEntry, error) {
	rows, err := sqlcgen.New(txscope.New(pool)).WeaveAuditReleaseProjectDependencies(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit release project dependencies: %w", err)
	}
	out := make([]DependencyAuditEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, DependencyAuditEntry{ProjectID: row.ProjectID, Version: row.Version, CreatedAt: row.CreatedAt, ArchiveOwnershipVersion: row.ArchiveOwnershipVersion})
	}
	return out, nil
}
