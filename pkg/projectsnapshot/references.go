package projectsnapshot

import (
	"fmt"
	"strings"
)

func (g *graph) resolve(owner Identity, kind, id string) (nodeKey, error) {
	key := EntityKey{Source: owner, Kind: kind, ID: id}
	if node, ok := g.entities[key]; ok {
		return node, nil
	}
	if source, ok := g.projects[owner].members[localKey{kind, id}]; ok {
		if node, found := g.entities[EntityKey{Source: source, Kind: kind, ID: id}]; found {
			return node, nil
		}
	}
	return nodeKey{}, fmt.Errorf("%s %s is absent from %s's selected content", kind, id, owner.project)
}

func (g *graph) link(from nodeKey, kind, id string, optional, parent bool) error {
	if strings.TrimSpace(id) == "" {
		if optional {
			return nil
		}
		return fmt.Errorf("%s has an empty required %s reference", from.section, kind)
	}
	to, err := g.resolve(from.source, kind, id)
	if err != nil {
		return err
	}
	if parent && to.source != from.source && ownedChild(from.section) {
		return fmt.Errorf("owned child has a foreign parent")
	}
	g.edges[from] = append(g.edges[from], to)
	if parent {
		g.edges[to] = append(g.edges[to], from)
	}
	return nil
}

func ownedChild(section string) bool {
	switch section {
	case sectionOverrideRefs, sectionExampleValues, sectionConceptListEntries, sectionConceptBroader, sectionVocabularyEntries:
		return true
	default:
		return false
	}
}

func (g *graph) connect() error {
	for _, id := range g.identities() {
		p := g.projects[id]
		for _, section := range Coverage() {
			for _, key := range p.rows[section.Name] {
				if err := g.connectRow(key, g.nodes[key]); err != nil {
					return err
				}
			}
		}
		if err := g.checkInterpretation(id); err != nil {
			return err
		}
	}
	return nil
}

type reference struct {
	kind, column     string
	optional, parent bool
}

func (g *graph) connectRow(key nodeKey, r record) error {
	var err error
	switch key.section {
	case sectionProjects:
		if r.text(columnOwnerID) == "" || r.text("effective_concept_namespace") == "" || r.text("enforce_concept_lists") == "" {
			return fmt.Errorf("project lacks owner or effective concept policy")
		}
	case "weave_field_overrides_archive":
		err = g.connectOverride(key, r)
	case sectionOverrideRefs:
		err = g.connectOverrideRef(key, r)
	case sectionExampleValues:
		err = g.connectSavedValue(key, r)
	case "weave_project_inheritance_archive":
		err = g.connectInheritance(key, r)
	case sectionAdoptions:
		err = g.connectAdoption(key, r)
	}
	if err != nil {
		return err
	}
	for _, ref := range r.references(key.section) {
		if err := g.link(key, ref.kind, r.text(ref.column), ref.optional, ref.parent); err != nil {
			return err
		}
	}
	return g.checkActors(key, r)
}

func (r record) references(section string) []reference {
	switch section {
	case "weave_collections_archive":
		return []reference{{kindCategory, "default_category_id", true, false}}
	case "weave_collection_placements_archive":
		return []reference{{kindModel, "model_id", false, true}, {kindCollection, "collection_id", false, false}, {kindCategory, columnCategoryID, true, false}}
	case "weave_examples_archive":
		return []reference{{r.text(columnEntityType), columnEntityID, false, false}}
	case "weave_concept_lists_archive":
		return []reference{{kindVocabulary, columnVocabularyID, true, false}, {kindVocabularyEntry, "list_type", true, false}}
	case sectionConceptListEntries:
		return []reference{{kindConceptList, "concept_list_id", false, true}, {kindVocabularyEntry, "vocabulary_entry_id", false, false}}
	case sectionConceptBroader:
		return []reference{{kindConceptList, "scheme_id", false, true}, {kindVocabularyEntry, "concept_id", false, false}, {kindVocabularyEntry, "broader_id", false, false}}
	case sectionVocabularyEntries:
		return []reference{{kindVocabulary, columnVocabularyID, false, true}}
	default:
		return nil
	}
}

func (g *graph) connectOverride(key nodeKey, r record) error {
	refs := []reference{{kindField, columnFieldID, false, r.text(columnEntityType) == ""}, {kindCategory, columnCategoryID, true, false}, {kindCollection, columnPartOfCollectionID, true, false}, {kindVocabularyEntry, "set_value_entry_id", true, false}}
	// A consumer-owned base override points at its adopted source body; it does
	// not become an owned child of that source project's field.
	if _, ok := g.projects[key.source].members[localKey{kindField, r.text(columnFieldID)}]; ok {
		refs[0].parent = false
	}
	if r.text(columnEntityType) != "" {
		refs = append(refs, reference{r.text(columnEntityType), columnEntityID, false, true})
	}
	for _, ref := range refs {
		if err := g.link(key, ref.kind, r.text(ref.column), ref.optional, ref.parent); err != nil {
			return err
		}
	}
	return nil
}

func (g *graph) connectOverrideRef(key nodeKey, r record) error {
	if err := g.link(key, kindOverride, r.text(columnOverrideID), false, true); err != nil {
		return err
	}
	kind := r.text(columnRefType)
	switch kind {
	case "resource_model":
		kind = kindModel
	case "collection_model":
		kind = kindCollection
	}
	target := r.text("target_id")
	if target == "" {
		target = r.text(columnSemanticID)
	}
	return g.link(key, kind, target, false, false)
}

func (g *graph) connectSavedValue(key nodeKey, r record) error {
	refs := []reference{{kindExample, "example_id", false, true}, {kindExample, "linked_example_id", true, false}}
	// Deleted placement/field references remain stale evidence. Linked examples
	// still require bodies; a missing link is not a stale-placement exception.
	_, fieldErr := g.resolve(key.source, kindField, r.text(columnFieldID))
	_, overrideErr := g.resolve(key.source, kindOverride, r.text(columnOverrideID))
	if fieldErr == nil && overrideErr == nil {
		refs = append(refs, reference{kindField, columnFieldID, false, false}, reference{kindOverride, columnOverrideID, false, false}, reference{kindCollection, columnPartOfCollectionID, true, false})
	}
	for _, ref := range refs {
		if err := g.link(key, ref.kind, r.text(ref.column), ref.optional, ref.parent); err != nil {
			return err
		}
	}
	return nil
}

func (g *graph) connectInheritance(key nodeKey, r record) error {
	if r.text("source_mode") == "draft" {
		return fmt.Errorf("complete content cannot follow a source draft")
	}
	to, err := g.resolve(key.source, kindProject, r.text(columnParentProjectID))
	if err != nil {
		return err
	}
	if v := r.text(columnSourceVersion); v != "" && v != to.source.version {
		return fmt.Errorf("inheritance pin differs from selected source")
	}
	g.edges[key] = append(g.edges[key], to)
	return nil
}

func (g *graph) connectAdoption(key nodeKey, r record) error {
	to, err := g.resolve(key.source, r.text(columnEntityType), r.text(columnSourceEntityID))
	if err != nil {
		return err
	}
	if to.source.project != r.text(columnSourceProjectID) || r.text(columnSourceVersion) != "" && to.source.version != r.text(columnSourceVersion) {
		return fmt.Errorf("adoption differs from selected source")
	}
	g.edges[key] = append(g.edges[key], to)
	if r.text(columnContextEntityType) != "" {
		return g.link(key, r.text(columnContextEntityType), r.text(columnContextEntityID), false, true)
	}
	return nil
}

func (g *graph) checkActors(key nodeKey, r record) error {
	for _, name := range []string{columnOwnerID, columnCreatedByID, columnActorID, "added_by_id"} {
		id := r.text(name)
		if id != "" {
			if _, err := g.projects[key.source].evidence.descriptors.Actor(&id); err != nil {
				return err
			}
		}
	}
	return nil
}
