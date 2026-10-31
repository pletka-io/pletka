package projectread

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

type entityContext struct {
	session   *readsession.Session
	input     projectsnapshot.ProjectInput
	selection projectsnapshot.Selection
	dirty     bool
}

// Entity loads only the requested structural closure. Interpretation metadata
// and source inventories remain complete. Paging and every closure expansion
// share the supplied session; there is no Project-then-filter fallback.
func (a *Assembler) Entity(ctx context.Context, s *readsession.Session, kind, id string) (projectsnapshot.EntityFragment, error) {
	if a == nil {
		return projectsnapshot.EntityFragment{}, fmt.Errorf("project assembler is uninitialized")
	}
	if err := a.options.Limits.validate(); err != nil {
		return projectsnapshot.EntityFragment{}, err
	}
	if err := s.Check(ctx); err != nil {
		return projectsnapshot.EntityFragment{}, err
	}
	contexts, err := a.entityContexts(ctx, s)
	if err != nil {
		return projectsnapshot.EntityFragment{}, err
	}
	if err := requestEntity(contexts, 0, kind, id); err != nil {
		return projectsnapshot.EntityFragment{}, err
	}
	b := budget{limits: a.options.Limits}
	if err := loadEntityClosure(ctx, contexts, &b); err != nil {
		return projectsnapshot.EntityFragment{}, err
	}
	root, sources, err := a.entityEvidence(ctx, contexts, &b)
	if err != nil {
		return projectsnapshot.EntityFragment{}, err
	}
	value, err := projectsnapshot.NewFragment(root, sources, kind, id)
	if err != nil {
		if s.Scope().IsRelease() {
			err = errors.Join(&readsession.NotInReleaseError{ProjectID: s.ProjectID(), Version: s.Scope().Version(), EntityID: id}, err)
		}
		return projectsnapshot.EntityFragment{}, err
	}
	if err := ctx.Err(); err != nil {
		return projectsnapshot.EntityFragment{}, err
	}
	return value, nil
}

func (a *Assembler) entityContexts(ctx context.Context, s *readsession.Session) ([]*entityContext, error) {
	deps, err := s.Dependencies(ctx)
	if err != nil {
		return nil, err
	}
	if len(deps) >= a.options.Limits.MaxProjects {
		return nil, &BudgetError{Resource: "projects"}
	}
	sessions := []*readsession.Session{s}
	for _, dep := range deps {
		bound, err := s.ProjectDependency(ctx, dep.ProjectID)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, bound)
	}
	result := make([]*entityContext, 0, len(sessions))
	for _, bound := range sessions {
		c := &entityContext{session: bound, selection: projectsnapshot.Selection{}, dirty: true}
		c.input.Dependencies = []projectsnapshot.Identity{}
		c.input.Membership = []projectsnapshot.Membership{}
		deps, err := bound.Dependencies(ctx)
		if err != nil {
			return nil, err
		}
		for _, dep := range deps {
			id, err := projectsnapshot.Release(dep.ProjectID, dep.Version)
			if err != nil {
				return nil, err
			}
			c.input.Dependencies = append(c.input.Dependencies, id)
		}
		members, err := bound.Membership(ctx)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			id, err := projectsnapshot.Release(m.Source.ProjectID, m.Source.Version)
			if err != nil {
				return nil, err
			}
			c.input.Membership = append(c.input.Membership, projectsnapshot.Membership{Kind: m.Kind, ID: m.ID, Source: id})
		}
		result = append(result, c)
	}
	return result, nil
}

func requestEntity(contexts []*entityContext, index int, kind, id string) error {
	c := contexts[index]
	changed, err := c.selection.Include(kind, id)
	if err != nil {
		return err
	}
	c.dirty = c.dirty || changed
	for _, m := range c.input.Membership {
		if m.Kind != kind || m.ID != id {
			continue
		}
		for _, source := range contexts {
			if source.session.ProjectID() != m.Source.ProjectID() {
				continue
			}
			changed, err := source.selection.Include(kind, id)
			if err != nil {
				return err
			}
			source.dirty = source.dirty || changed
			return nil
		}
		return fmt.Errorf("entity source is outside authorized closure")
	}
	return nil
}

// A selected source field may have a consumer-owned base commitment even when
// reached through a source model. Load it only in contexts this request uses.
func requestConsumerDefaults(contexts []*entityContext) error {
	for i, c := range contexts {
		if i > 0 && len(c.selection) == 0 {
			continue
		}
		for _, m := range c.input.Membership {
			if m.Kind != "field" {
				continue
			}
			for _, source := range contexts {
				if source.session.ProjectID() != m.Source.ProjectID() {
					continue
				}
				if slices.ContainsFunc(source.input.Content.Fields, func(f projectsnapshot.FieldRecord) bool { return f.ID == m.ID }) {
					if err := requestEntity(contexts, i, m.Kind, m.ID); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func loadEntityClosure(ctx context.Context, contexts []*entityContext, b *budget) error {
	var err error
	for pending := true; pending; {
		pending = false
		for _, c := range contexts {
			if !c.dirty {
				continue
			}
			c.dirty = false
			b.selection = c.selection
			c.input.Content, c.input.Counts, err = loadContent(ctx, c.session, b)
			if err != nil {
				return err
			}
		}
		for i, c := range contexts {
			requests, err := projectsnapshot.RequiredEntities(c.input.Content)
			if err != nil {
				return err
			}
			kinds := make([]string, 0, len(requests))
			for k := range requests {
				kinds = append(kinds, k)
			}
			slices.Sort(kinds)
			for _, k := range kinds {
				for _, entity := range requests[k] {
					if err := requestEntity(contexts, i, k, entity); err != nil {
						return err
					}
				}
			}
		}
		if err := requestConsumerDefaults(contexts); err != nil {
			return err
		}
		for _, c := range contexts {
			pending = pending || c.dirty
		}
	}

	return nil
}

func (a *Assembler) entityEvidence(ctx context.Context, contexts []*entityContext, b *budget) (projectsnapshot.ProjectInput, []projectsnapshot.ProjectInput, error) {
	var err error
	s := contexts[0].session
	sources := make([]projectsnapshot.ProjectInput, 0, len(contexts)-1)
	for _, c := range contexts[1:] {
		c.input.Evidence, err = releasedEvidence(ctx, c.session)
		if err != nil {
			return projectsnapshot.ProjectInput{}, nil, err
		}
		if err := b.evidence(c.input.Evidence); err != nil {
			return projectsnapshot.ProjectInput{}, nil, err
		}
		sources = append(sources, c.input)
	}
	root := contexts[0].input
	if s.Scope().IsRelease() {
		root.Evidence, err = releasedEvidence(ctx, s)
	} else {
		for i := range root.Content.Adoptions {
			bound, e := s.ProjectDependency(ctx, root.Content.Adoptions[i].SourceProjectID)
			if e != nil {
				return projectsnapshot.ProjectInput{}, nil, e
			}
			root.Content.Adoptions[i].SourceVersion = bound.Scope().Version()
		}
		root.Evidence, err = a.draftEvidence(ctx, s, root.Content, sources)
	}
	if err != nil {
		return projectsnapshot.ProjectInput{}, nil, err
	}
	if err := b.evidence(root.Evidence); err != nil {
		return projectsnapshot.ProjectInput{}, nil, err
	}

	return root, sources, nil
}
