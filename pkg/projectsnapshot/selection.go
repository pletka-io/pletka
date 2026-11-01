package projectsnapshot

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// Selection restricts structural rows by entity identity within an already bound
// project. Nil selects complete content; an explicit empty map selects metadata.
// Child rows are loaded by their selected parent. It never chooses a source.
type Selection map[string][]string

// Validate rejects unsupported kinds and empty identities before querying.
func (s Selection) Validate() error {
	for kind, ids := range s {
		if !selectable(kind) {
			return fmt.Errorf("unsupported selection kind %q", kind)
		}
		for _, id := range ids {
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("empty selected identity")
			}
		}
	}
	return nil
}

func selectable(kind string) bool {
	switch kind {
	case kindField, kindModel, kindCollection, kindCategory, kindOverride, kindExample, kindConceptList, kindVocabulary, kindVocabularyEntry:
		return true
	default:
		return false
	}
}

// Include returns whether this selection grew. The assembler owns the map.
func (s Selection) Include(kind, id string) (bool, error) {
	if s == nil || !selectable(kind) || strings.TrimSpace(id) == "" {
		return false, fmt.Errorf("invalid entity selection %s %s", kind, id)
	}
	if slices.Contains(s[kind], id) {
		return false, nil
	}
	s[kind] = append(s[kind], id)
	slices.Sort(s[kind])
	return true, nil
}

// RequiredEntities enumerates structural closure requests in this content's own
// context. Membership, not ID spelling, determines the actual source. Saved-value
// field/override requests may be absent; final graph validation applies the stale
// value exception only when either body is absent.
func RequiredEntities(c Content) (Selection, error) {
	out := Selection{}
	err := visitContent(&c, func(section Section, rows reflect.Value) error {
		for i := range rows.Len() {
			r, err := freezeRow(rows.Index(i))
			if err != nil {
				return err
			}
			refs := r.references(section.Name)
			if selectable(section.Kind) {
				refs = append(refs, reference{kind: section.Kind, column: columnID})
			}
			switch section.Name {
			case sectionFieldOverrides:
				refs = append(refs, reference{kind: kindField, column: columnFieldID}, reference{kind: kindCategory, column: columnCategoryID}, reference{kind: kindCollection, column: columnPartOfCollectionID}, reference{kind: kindVocabularyEntry, column: columnSetValueEntryID})
				if r.text(columnEntityType) != "" {
					refs = append(refs, reference{kind: r.text(columnEntityType), column: columnEntityID})
				}
			case sectionOverrideRefs:
				kind := r.text(columnRefType)
				if kind == refResourceModel {
					kind = kindModel
				}
				if kind == refCollectionModel {
					kind = kindCollection
				}
				column := "target_id"
				if r.text(column) == "" {
					column = columnSemanticID
				}
				refs = append(refs, reference{kind: kindOverride, column: columnOverrideID}, reference{kind: kind, column: column})
			case sectionExampleValues:
				refs = append(refs, reference{kind: kindExample, column: columnExampleID}, reference{kind: kindExample, column: columnLinkedExampleID}, reference{kind: kindField, column: columnFieldID}, reference{kind: kindOverride, column: columnOverrideID}, reference{kind: kindCollection, column: columnPartOfCollectionID})
			case sectionAdoptions:
				refs = append(refs, reference{kind: r.text(columnEntityType), column: columnSourceEntityID})
			}
			for _, ref := range refs {
				id := r.text(ref.column)
				if id == "" || id == "0" {
					continue
				}
				if _, err := out.Include(ref.kind, id); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
