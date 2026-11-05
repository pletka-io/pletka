package release

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/releasedeps"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// preserveLegacyInTx re-owns a release's archived rows and rebuilds only its
// evidence. Order matters: the import-ledger row is written after the ontology
// snapshot and descriptors (their triggers then take ordinary branches) and
// before finalization (whose metadata and content guards then take their
// import branches and skip the hot-table comparisons).
func (s *Service) preserveLegacyInTx(ctx context.Context, tx pgx.Tx, p RebuildProject, actorID, operation string) error {
	q := sqlcgen.New(tx)
	count, err := q.WeavePreservedArchiveState(ctx, sqlcgen.WeavePreservedArchiveStateParams{ProjectID: p.ProjectID, Version: p.Version})
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("expected one archived project row, found %d", count)
	}
	if err := q.WeavePreserveReleaseRow(ctx, sqlcgen.WeavePreserveReleaseRowParams{
		ProjectID: p.ProjectID, Version: p.Version, Title: p.Title, Description: p.Description,
		CreatedAt: p.CreatedAt, CreatedByID: p.CreatedByID,
	}); err != nil {
		return fmt.Errorf("restore release row: %w", err)
	}
	// Bindings are prefix policy, captured from the current policy; every other
	// section is whatever the archive holds, including nothing.
	if err := q.WeaveSnapshotReleaseNamespaces(ctx, sqlcgen.WeaveSnapshotReleaseNamespacesParams{ProjectID: p.ProjectID, Version: p.Version}); err != nil {
		return fmt.Errorf("capture namespaces: %w", err)
	}
	if err := releasedeps.CaptureArchived(ctx, q, p.ProjectID, p.Version); err != nil {
		var unavailable *releasedeps.UnavailableError
		if errors.As(err, &unavailable) {
			return &ErrValidation{Fields: map[string][]string{"dependencies": {unavailable.Error()}}}
		}
		return fmt.Errorf("capture archived project dependencies: %w", err)
	}
	if err := q.WeavePinArchivedAdoptions(ctx, sqlcgen.WeavePinArchivedAdoptionsParams{ProjectID: p.ProjectID, Version: p.Version}); err != nil {
		return fmt.Errorf("pin archived adoptions: %w", err)
	}
	if err := (legacyEvidence{}).capture(ctx, q, p.ProjectID, p.Version, s.hosts); err != nil {
		return evidenceError(err)
	}
	if err := q.WeaveInsertPreservedReleaseImport(ctx, sqlcgen.WeaveInsertPreservedReleaseImportParams{ProjectID: p.ProjectID, Version: p.Version, Operation: operation}); err != nil {
		return fmt.Errorf("record preserved archive: %w", err)
	}
	if err := q.WeaveInsertReleaseChangeSet(ctx, sqlcgen.WeaveInsertReleaseChangeSetParams{ProjectID: p.ProjectID, ActorID: actorID, CommitMessage: "Release v" + p.Version, Version: p.Version}); err != nil {
		return fmt.Errorf("enqueue release change set: %w", err)
	}
	if err := (legacyEvidence{}).finalize(ctx, q, p.ProjectID, p.Version); err != nil {
		return fmt.Errorf("mark release archive ownership: %w", err)
	}
	return nil
}
