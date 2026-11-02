//go:build integration

package projectread

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// EntityUsingRowsForTest retains the previous traversal as an independent oracle.
// This helper is compiled only into this package's integration test binary.
// It uses full row readers (including harmless metadata reads); only closure
// semantics are compared, not the previous traversal's workload counters.
func EntityUsingRowsForTest(ctx context.Context, a *Assembler, s *readsession.Session, kind, id string) (projectsnapshot.EntityFragment, error) {
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
	if err := loadEntityClosureRows(ctx, contexts, &b); err != nil {
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

func loadEntityClosureRows(ctx context.Context, contexts []*entityContext, b *budget) error {
	var err error
	for pending := true; pending; {
		pending = false
		for _, c := range contexts {
			// Only this round's rows contribute new closure requests.
			c.input.Content = projectsnapshot.Content{}
			if !c.dirty {
				continue
			}
			c.dirty = false
			b.selection = nextSelection(c.selection, c.discovered)
			c.input.Content, c.input.Counts, err = loadContent(ctx, c.session, b)
			if err != nil {
				return err
			}
			children := make([]string, len(c.input.Content.VocabularyEntries))
			for i, row := range c.input.Content.VocabularyEntries {
				children[i] = row.ID
			}
			if err := discoveredChildren(children, c.discovered); err != nil {
				return err
			}
			for _, field := range c.input.Content.Fields {
				c.fields[field.ID] = true
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
	// Materialize the closed selection once in database order. Discovery never
	// reads the growing union again, and metadata is fetched only here. Counting
	// both passes in the budget reflects actual query work, including overlaps.
	for _, c := range contexts {
		b.selection = c.selection
		c.input.Content, c.input.Counts, err = loadContent(ctx, c.session, b)
		if err != nil {
			return err
		}
		if err := checkDiscovered(c.input.Content, c.selection); err != nil {
			return err
		}
	}
	return nil
}

// DiscoveryRowsForTest compares one SQL expansion against the previous typed rows.
func DiscoveryRowsForTest(ctx context.Context, s *readsession.Session, selection projectsnapshot.Selection) (projectsnapshot.Selection, projectsnapshot.Selection, error) {
	b := budget{limits: DefaultOptions().Limits, selection: selection}
	content, _, err := loadContent(ctx, s, &b)
	if err != nil {
		return nil, nil, err
	}
	requested, err := projectsnapshot.RequiredEntities(content)
	if err != nil {
		return nil, nil, err
	}
	present := projectsnapshot.Selection{}
	sections := map[string]string{}
	for _, section := range projectsnapshot.Coverage() {
		sections[section.Name] = section.Kind
	}
	value := reflect.ValueOf(content)
	for _, binding := range Bindings() {
		kind := sections[binding.Section]
		if kind == "" || kind == "project" {
			continue
		}
		rows := value.FieldByName(binding.Field)
		for i := 0; i < rows.Len(); i++ {
			id := fmt.Sprint(rows.Index(i).FieldByName("ID").Interface())
			if id == "" || id == "0" {
				continue
			}
			if _, err := present.Include(kind, id); err != nil {
				return nil, nil, err
			}
		}
	}
	return requested, present, nil
}
