package readsession

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/releasedeps"
)

// SourceAuthorization applies request or job policy to current access metadata.
// It is configured once on the factory and inherited by every source session.
// It must not be inferred from a content pin or captured visibility.
type SourceAuthorization func(context.Context, auth.Resource) error

// WithSourceAuthorization returns an independently configured factory. The
// default factory can read its authorized root but cannot traverse sources.
// Interactive callers can supply auth.CheckProjectRead; trusted jobs must supply
// their explicit policy. The caller still authorizes the root before Read.
func (f *Factory) WithSourceAuthorization(policy SourceAuthorization) *Factory {
	if f == nil {
		return &Factory{authorize: policy}
	}
	copy := *f
	copy.authorize = policy
	return &copy
}

// Source is a detached, concrete dependency identity. It grants no access.
type Source = releasedeps.Source

type sourceGraph struct {
	authorize func(context.Context, string) error
	open      func(context.Context, Source) (*Session, error)
}

type dependencies struct {
	graph  *sourceGraph
	load   func(context.Context) (releasedeps.Plan, error)
	loaded bool
	plan   releasedeps.Plan
	err    error
}

func (s *Session) dependencyPlan(ctx context.Context) (releasedeps.Plan, error) {
	if s == nil || s.lifetime == nil || s.lifetime.closed || s.dependencies == nil || s.dependencies.graph.authorize == nil {
		return releasedeps.Plan{}, fmt.Errorf("dependency reads require an open session and source authorization")
	}
	if err := ctx.Err(); err != nil {
		return releasedeps.Plan{}, err
	}
	d := s.dependencies
	if !d.loaded {
		d.plan, d.err = d.load(ctx)
		d.loaded = true
	}
	return d.plan, d.err
}

// Dependencies returns the complete selected source-project closure. Drafts
// plan once using explicit pins then latest releases; released roots use only
// their captured inventory. No values escape unless every source is authorized.
func (s *Session) Dependencies(ctx context.Context) ([]Source, error) {
	plan, err := s.dependencyPlan(ctx)
	if err != nil {
		return nil, err
	}
	sources := plan.Sources()
	for _, source := range sources {
		if err := s.dependencies.graph.authorize(ctx, source.ProjectID); err != nil {
			return nil, err
		}
	}
	return sources, nil
}

// ProjectDependency binds only a member of the selected project closure, on
// this transaction and lifetime. Callers cannot supply a version or a draft.
func (s *Session) ProjectDependency(ctx context.Context, project string) (*Session, error) {
	plan, err := s.dependencyPlan(ctx)
	if err != nil {
		return nil, err
	}
	source, ok := plan.Project(project)
	if !ok {
		return nil, &NotInReleaseError{ProjectID: s.projectID, Version: s.scope.Version(), EntityID: "dependency project " + project}
	}
	return s.dependencies.graph.open(ctx, source)
}

// Dependency binds an entity's recorded or planned source release. Authorization
// and source selection are central, inherited capabilities, never caller flags.
func (s *Session) Dependency(ctx context.Context, kind, id string) (*Session, error) {
	plan, err := s.dependencyPlan(ctx)
	if err != nil {
		return nil, err
	}
	source, ok := plan.Entity(kind, id)
	if !ok {
		return nil, &NotInReleaseError{ProjectID: s.projectID, Version: s.scope.Version(), EntityID: kind + " " + id}
	}
	return s.dependencies.graph.open(ctx, source)
}
