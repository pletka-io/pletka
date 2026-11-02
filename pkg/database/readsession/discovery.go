package readsession

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/readsql/archive"
	"github.com/pletka-io/pletka/pkg/database/readsql/draft"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// DiscoveryIdentity is one structural request in this bound project's context.
// Present distinguishes an actual body from a reference that may be absent.
// These are acquisition details, not serialized generator or product inputs.
type DiscoveryIdentity struct {
	Kind, ID string
	Present  bool
}

// DiscoverIdentities expands one explicit selection without choosing sources.
// It deduplicates typed requests, preserves actual body presence, and loads no metadata.
func (s *Session) DiscoverIdentities(ctx context.Context, page Page) ([]DiscoveryIdentity, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	if err := page.Validate(); err != nil {
		return nil, err
	}
	if page.Selection == nil {
		return nil, fmt.Errorf("discovery requires an explicit selection")
	}
	selected := false
	for _, ids := range page.Selection {
		selected = selected || len(ids) > 0
	}
	out := []DiscoveryIdentity{}
	if !selected {
		return out, nil
	}
	add := func(kind, id string, present bool) error {
		if err := (projectsnapshot.Selection{kind: {id}}).Validate(); err != nil {
			return err
		}
		out = append(out, DiscoveryIdentity{Kind: kind, ID: id, Present: present})
		return nil
	}
	if s.Scope().IsRelease() {
		q, err := s.ArchiveQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.DiscoverContentIdentities(ctx, archive.DiscoverContentIdentitiesParams{
			ProjectID: s.ProjectID(), Version: s.Scope().Version(), PageOffset: page.Offset, PageLimit: page.Limit,
			SelectedModel: page.IDs("model"), SelectedCollection: page.IDs("collection"), SelectedField: page.IDs("field"),
			SelectedCategory: page.IDs("category"), SelectedConceptList: page.IDs("concept_list"), SelectedExample: page.IDs("example"),
			SelectedOverride: page.IDs("override"), SelectedVocabulary: page.IDs("vocabulary"), SelectedVocabularyEntry: page.IDs("vocabulary_entry"),
		})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if err := add(row.Kind, row.ID, row.Present); err != nil {
				return nil, err
			}
		}
	} else {
		q, err := s.DraftQueries()
		if err != nil {
			return nil, err
		}
		rows, err := q.DiscoverContentIdentities(ctx, draft.DiscoverContentIdentitiesParams{
			ProjectID: s.ProjectID(), PageOffset: page.Offset, PageLimit: page.Limit,
			SelectedModel: page.IDs("model"), SelectedCollection: page.IDs("collection"), SelectedField: page.IDs("field"),
			SelectedCategory: page.IDs("category"), SelectedConceptList: page.IDs("concept_list"), SelectedExample: page.IDs("example"),
			SelectedOverride: page.IDs("override"), SelectedVocabulary: page.IDs("vocabulary"), SelectedVocabularyEntry: page.IDs("vocabulary_entry"),
		})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if err := add(row.Kind, row.ID, row.Present); err != nil {
				return nil, err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
