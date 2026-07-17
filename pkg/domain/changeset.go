package domain

import (
	"context"
	"time"
)

// ChangeSet represents a group of related changes made by an actor within a project.
type ChangeSet struct {
	ID            int64
	ProjectID     string
	ActorID       string
	ActorName     string
	ActorEmail    string
	CommitMessage string
	StartedAt     time.Time
	ClosedAt      *time.Time
}

// ChangeSetOptions provides the parameters needed to create a new ChangeSet.
type ChangeSetOptions struct {
	ProjectID     string
	CommitMessage string
	ActorID       string
	ActorName     string
	ActorEmail    string
}

type changeSetKey struct{}

// ContextWithChangeSet returns a new context carrying the given ChangeSet.
func ContextWithChangeSet(ctx context.Context, cs *ChangeSet) context.Context {
	return context.WithValue(ctx, changeSetKey{}, cs)
}

// ChangeSetFromContext extracts the ChangeSet from the context, or nil if absent.
func ChangeSetFromContext(ctx context.Context) *ChangeSet {
	cs, _ := ctx.Value(changeSetKey{}).(*ChangeSet)
	return cs
}

// ChangeSetHint carries request-scoped identity used to stamp an implicit
// change set (project + actor) the first time a mutation opens one. Request
// middleware seeds it; resolveChangeSet reads it so materialized commits route
// to the right project repo (baseDir/<project_id>) and carry real authorship
// instead of the "" project / "pletka-system" system fallback.
type ChangeSetHint struct {
	ProjectID     string
	ActorID       string
	ActorName     string
	ActorEmail    string
	CommitMessage string
}

type changeSetHintKey struct{}

// ContextWithChangeSetHint returns a new context carrying h.
func ContextWithChangeSetHint(ctx context.Context, h ChangeSetHint) context.Context {
	return context.WithValue(ctx, changeSetHintKey{}, h)
}

// ChangeSetHintFromContext extracts the ChangeSetHint, or ok=false if absent.
func ChangeSetHintFromContext(ctx context.Context) (ChangeSetHint, bool) {
	h, ok := ctx.Value(changeSetHintKey{}).(ChangeSetHint)
	return h, ok
}
