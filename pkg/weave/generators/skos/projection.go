package skos

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// RenderFragment projects a selected concept list into the existing SKOS format.
// All labels, memberships, edges and the list owner's namespace are retained;
// rendering has no source, transaction or live vocabulary capability.
func RenderFragment(w io.Writer, value projectsnapshot.EntityFragment) error {
	root, err := value.Root()
	if err != nil {
		return err
	}
	if root.Kind != "concept_list" {
		return fmt.Errorf("SKOS requires a concept-list fragment")
	}
	content, err := value.Content(root.Source)
	if err != nil {
		return err
	}
	if len(content.Projects) != 1 || content.Projects[0].EffectiveConceptNamespace == nil || strings.TrimSpace(*content.Projects[0].EffectiveConceptNamespace) == "" {
		return fmt.Errorf("SKOS requires the selected list owner's concept namespace")
	}
	scheme, err := projectScheme(content, root.ID)
	if err != nil {
		return err
	}
	concepts, err := projectConcepts(value, root, content)
	if err != nil {
		return err
	}
	var edges []domain.ConceptBroaderEdge
	for _, row := range content.Broader {
		if row.SchemeID == root.ID {
			edges = append(edges, domain.ConceptBroaderEdge{ID: row.ID, ConceptID: row.ConceptID, BroaderID: row.BroaderID, SchemeID: &row.SchemeID, Position: int(row.Position)})
		}
	}
	return Render(w, *scheme, concepts, edges, map[string]string{"pletka": *content.Projects[0].EffectiveConceptNamespace})
}

func projectScheme(content projectsnapshot.Content, id string) (*domain.ConceptList, error) {
	var scheme *domain.ConceptList
	for _, row := range content.ConceptLists {
		if row.ID != id {
			continue
		}
		scheme = &domain.ConceptList{Entity: domain.Entity{ID: row.ID}, IsClosed: row.IsClosed}
		if row.SemanticID != nil {
			scheme.SemanticID = *row.SemanticID
		}
		if err := decodeLabels(row.UIName, &scheme.UIName); err != nil {
			return nil, err
		}
	}
	if scheme == nil {
		return nil, fmt.Errorf("selected concept list is absent from fragment")
	}

	return scheme, nil
}

func projectConcepts(value projectsnapshot.EntityFragment, root projectsnapshot.EntityKey, content projectsnapshot.Content) ([]domain.VocabularyEntry, error) {
	entries := map[projectsnapshot.Identity]map[string]projectsnapshot.VocabularyEntryRecord{}
	concepts := make([]domain.VocabularyEntry, 0, len(content.ConceptEntries))
	for _, member := range content.ConceptEntries {
		if member.ConceptListID != root.ID {
			continue
		}
		key, err := value.Resolve(root.Source, "vocabulary_entry", member.VocabularyEntryID)
		if err != nil {
			return nil, err
		}
		if entries[key.Source] == nil {
			source, err := value.Content(key.Source)
			if err != nil {
				return nil, err
			}
			entries[key.Source] = make(map[string]projectsnapshot.VocabularyEntryRecord, len(source.VocabularyEntries))
			for _, row := range source.VocabularyEntries {
				entries[key.Source][row.ID] = row
			}
		}
		row, ok := entries[key.Source][key.ID]
		if !ok {
			return nil, fmt.Errorf("selected concept %s is absent from fragment", key.ID)
		}
		concept := domain.VocabularyEntry{ID: row.ID, URI: row.URI}
		var custom domain.Translations
		for _, labels := range []struct {
			raw []byte
			out *domain.Translations
		}{{row.Label, &concept.Label}, {row.ScopeNote, &concept.ScopeNote}, {member.CustomLabel, &custom}} {
			if err := decodeLabels(labels.raw, labels.out); err != nil {
				return nil, err
			}
		}
		if len(custom) > 0 {
			concept.Label = custom
		}
		concepts = append(concepts, concept)
	}

	return concepts, nil
}

func decodeLabels(raw []byte, out *domain.Translations) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode retained SKOS labels: %w", err)
	}
	return nil
}
