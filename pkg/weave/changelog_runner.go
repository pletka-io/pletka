package weave

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// NewChangeLogRunner adapts PostgresStore.WithChangeLog to the
// domain.ChangeLogRunner interface so slice services persist change_log
// entries in a real transaction. It replaces domain.NoopChangeLogRunner in
// production; the persisted entries are drained by the gitmaterializer.
//
// The mutation write and the change_log flush are NOT one atomic unit — each
// slice Store method opens its own transaction, and WithChangeLog opens a
// separate one for the log flush. This is the DB-first policy: on flush
// failure the entity write stands and the git history lags until an operator
// retry. See the git-integration-live-save plan, "Partial Commit State".
func NewChangeLogRunner(store *PostgresStore) domain.ChangeLogRunner {
	return storeChangeLogRunner{store: store}
}

type storeChangeLogRunner struct{ store *PostgresStore }

func (r storeChangeLogRunner) Run(ctx context.Context, fn func(context.Context, domain.ChangeLogRecorder) error) error {
	return r.store.WithChangeLog(ctx, func(cl *ChangeLogger) error {
		return fn(ctx, changeLogRecorder{cl: cl})
	})
}

// changeLogRecorder bridges the ctx-carrying domain.ChangeLogRecorder to the
// tx-scoped ChangeLogger (whose Record takes no context).
type changeLogRecorder struct{ cl *ChangeLogger }

func (r changeLogRecorder) Record(_ context.Context, entry domain.ChangeLogEntry) error {
	return r.cl.Record(entry)
}
