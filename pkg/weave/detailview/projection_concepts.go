package detailview

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

func (p *detailProjection) conceptList(out *projectedDetail) {
	c := p.content[p.root.Source]
	for _, list := range c.ConceptLists {
		if list.ID != p.root.ID {
			continue
		}
		m := &out.Response.Entity
		m.ListType, m.VocabularyID = retainedValue(list.ListType), retainedValue(list.VocabularyID)
		out.Closed = list.IsClosed
		if m.VocabularyID != "" {
			key, err := p.fragment.Resolve(p.root.Source, "vocabulary", m.VocabularyID)
			if err != nil {
				p.err = err
				return
			}
			for _, v := range p.content[key.Source].Vocabularies {
				if v.ID == m.VocabularyID {
					name := retainedJSON[domain.Translations](p, v.UIName)
					m.VocabularyLabel = name.Get("en", retainedValue(v.SystemName))
					if m.VocabularyLabel == "" {
						m.VocabularyLabel = v.ID
					}
					m.SourceVocabulary = &domain.VocabularyRef{ID: v.ID, SemanticID: retainedValue(v.SemanticID), SystemName: retainedValue(v.SystemName), Name: name, BaseURI: retainedValue(v.BaseURI)}
					out.ConceptSource = m.VocabularyLabel
					out.RemoteSource = v.ConnectorType != "" && v.ConnectorType != "local"
				}
			}
		}
		if m.ListType != "" {
			e := p.concept(p.root.Source, m.ListType)
			m.ListTypeURI, m.ParentTermURI = e.URI, e.URI
			m.ListTypeLabel = e.Label.Get("en", e.URI)
			m.ParentTerm = &e
		}
		out.Response.Entries = []ConceptListEntryView{}
		for _, row := range c.ConceptEntries {
			if row.ConceptListID != list.ID {
				continue
			}
			e := p.concept(p.root.Source, row.VocabularyEntryID)
			out.Response.Entries = append(out.Response.Entries, ConceptListEntryView{ID: row.ID, VocabularyEntryID: row.VocabularyEntryID, CustomLabel: retainedJSON[domain.Translations](p, row.CustomLabel), Position: int(row.Position), URI: e.URI, Label: e.Label, ScopeNote: e.ScopeNote, ExternalID: e.ExternalID, BroaderURI: e.BroaderURI, BroaderPathItems: e.BroaderPath})
		}
		slices.SortFunc(out.Response.Entries, func(a, b ConceptListEntryView) int {
			if a.Position != b.Position {
				return a.Position - b.Position
			}
			return strings.Compare(a.ID, b.ID)
		})
		m.EntryCount = len(out.Response.Entries)
		return
	}
	p.err = fmt.Errorf("concept list is outside retained input")
}

func (p *detailProjection) concept(context projectsnapshot.Identity, id string) domain.VocabularyEntryRef {
	key, err := p.fragment.Resolve(context, "vocabulary_entry", id)
	if err != nil {
		p.err = err
		return domain.VocabularyEntryRef{}
	}
	for _, e := range p.content[key.Source].VocabularyEntries {
		if e.ID == key.ID {
			return domain.VocabularyEntryRef{ID: e.ID, VocabularyID: e.VocabularyID, URI: e.URI, Label: retainedJSON[domain.Translations](p, e.Label), ScopeNote: retainedJSON[domain.Translations](p, e.ScopeNote), ExternalID: retainedValue(e.ExternalID), BroaderURI: retainedValue(e.BroaderURI), BroaderPath: retainedJSON[[]domain.VocabularyEntryRef](p, e.BroaderPathItems)}
		}
	}
	p.err = fmt.Errorf("concept is outside retained input")
	return domain.VocabularyEntryRef{}
}
