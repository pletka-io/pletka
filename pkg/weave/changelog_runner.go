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
// Store adapters join the callback transaction through txscope. Entity writes,
// nested savepoints and audit flushing commit or roll back together. These domain
// entries still are not complete storage receipts; managed draft capture is a
// separate outer boundary.
func NewChangeLogRunner(store *PostgresStore) domain.ChangeLogRunner {
	return storeChangeLogRunner{store: store}
}

type storeChangeLogRunner struct{ store *PostgresStore }

func (r storeChangeLogRunner) Run(ctx context.Context, fn func(context.Context, domain.ChangeLogRecorder) error) error {
	return r.store.WithChangeLog(ctx, func(cl *ChangeLogger) error {
		return fn(cl.ctx, changeLogRecorder{cl: cl})
	})
}

// changeLogRecorder bridges the ctx-carrying domain.ChangeLogRecorder to the
// tx-scoped ChangeLogger (whose Record takes no context).
type changeLogRecorder struct{ cl *ChangeLogger }

func (r changeLogRecorder) Record(_ context.Context, entry domain.ChangeLogEntry) error {
	return r.cl.Record(entry)
}
