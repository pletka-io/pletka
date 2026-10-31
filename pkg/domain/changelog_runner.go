package domain

import "context"

// ChangeLogRecorder accepts ChangeLogEntry values during a transactional
// operation. Implementations buffer entries and flush them at commit time
// (production) or drop them silently (noop, used during the migration
// period before the slice changelog wiring is finished).
type ChangeLogRecorder interface {
	Record(ctx context.Context, entry ChangeLogEntry) error
}

// ChangeLogRunner supplies a ChangeLogRecorder while fn performs a mutation.
// Production store adapters join the callback transaction through its context:
// entity writes and audit flush commit or roll back together. Propagate that
// context to every store call.
// Entries are domain audit payloads, not lossless before/after storage images;
// they cannot be used as complete project draft patches.
//
// The current production implementation is in pkg/weave (TxRunner over
// PostgresStore.WithChangeLog). NoopChangeLogRunner is a drop-in
// replacement that performs the closure without recording anything —
// used during slice migration so the Service interface is final-shaped
// even before the audit trail is wired.
type ChangeLogRunner interface {
	Run(ctx context.Context, fn func(ctx context.Context, rec ChangeLogRecorder) error) error
}

// NoopChangeLogRunner returns a ChangeLogRunner that calls fn with a
// recorder that drops all entries. Use during migration when changelog
// wiring is incomplete; calls remain non-atomic with the underlying
// Store writes (each Store method opens its own tx).
func NoopChangeLogRunner() ChangeLogRunner {
	return noopRunner{}
}

type noopRunner struct{}

func (noopRunner) Run(ctx context.Context, fn func(context.Context, ChangeLogRecorder) error) error {
	return fn(ctx, noopRecorder{})
}

type noopRecorder struct{}

func (noopRecorder) Record(_ context.Context, _ ChangeLogEntry) error { return nil }
