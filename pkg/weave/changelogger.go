package weave

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5"
)

// ChangeLogger owns a transaction and records change_log entries as they
// occur. It is the sole entry point for mutations — the txn-scoped sqlc
// queries handle is only accessible through ChangeLogger.Queries().
type ChangeLogger struct {
	tx      pgx.Tx
	queries *sqlcgen.Queries
	csID    int64
	pending []domain.ChangeLogEntry
}

// Queries returns the transaction-scoped sqlc queries handle.
func (cl *ChangeLogger) Queries() *sqlcgen.Queries {
	return cl.queries
}

// ChangeSetID returns the change set this logger belongs to.
func (cl *ChangeLogger) ChangeSetID() int64 {
	return cl.csID
}

// Record appends a change log entry to the pending list. Entries are
// written to the database at transaction commit time.
//
// For "update" and "delete", previousPayload MUST be populated.
// For "create", previousPayload must be nil.
// For "delete", payload must be nil.
func (cl *ChangeLogger) Record(entry domain.ChangeLogEntry) error {
	switch entry.Operation {
	case "create":
		if entry.PreviousPayload != nil {
			return fmt.Errorf("create operation must not have previous_payload")
		}
		if entry.Payload == nil {
			return fmt.Errorf("create operation requires payload")
		}
	case "update":
		if entry.PreviousPayload == nil {
			return fmt.Errorf("update operation requires previous_payload")
		}
		if entry.Payload == nil {
			return fmt.Errorf("update operation requires payload")
		}
	case "delete":
		if entry.PreviousPayload == nil {
			return fmt.Errorf("delete operation requires previous_payload")
		}
		if entry.Payload != nil {
			return fmt.Errorf("delete operation must not have payload")
		}
	default:
		return fmt.Errorf("unknown operation: %s", entry.Operation)
	}

	cl.pending = append(cl.pending, entry)
	return nil
}

// writeChangeLogEntries writes all pending entries to the database.
func (cl *ChangeLogger) writeChangeLogEntries(ctx context.Context) error {
	for _, entry := range cl.pending {
		_, err := cl.queries.WeaveCreateChangeLogEntry(ctx, sqlcgen.WeaveCreateChangeLogEntryParams{
			ChangeSetID:     cl.csID,
			EntityType:      entry.EntityType,
			EntityID:        entry.EntityID,
			Operation:       entry.Operation,
			ProjectID:       entry.ProjectID,
			FilePath:        entry.FilePath,
			Payload:         entry.Payload,
			PreviousPayload: entry.PreviousPayload,
		})
		if err != nil {
			return fmt.Errorf("write change_log entry: %w", err)
		}
	}
	return nil
}
