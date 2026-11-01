package projectread

import (
	"fmt"
	"slices"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

const vocabularyEntriesSection = "weave_vocabulary_entries_archive"

// The selector dependencies of the content queries. Metadata is materialized
// once after discovery. An exhaustive SQL/coverage test guards this registry;
// unknown sections fail instead of silently skipping newly introduced edges.
func discoveryKinds() map[string][]string {
	const (
		model       = "model"
		collection  = "collection"
		field       = "field"
		override    = "override"
		vocabulary  = "vocabulary"
		category    = "category"
		conceptList = "concept_list"
		example     = "example"
	)
	return map[string][]string{
		"weave_adoptions_archive":                  {model, collection, field, category},
		"weave_categories_archive":                 {category},
		"weave_collection_placements_archive":      {model},
		"weave_collections_archive":                {collection},
		"weave_concept_broader_archive":            {conceptList},
		"weave_concept_list_entries_archive":       {conceptList},
		"weave_concept_lists_archive":              {conceptList},
		"weave_entity_forks_archive":               {model, collection, field},
		"weave_example_values_archive":             {example},
		"weave_examples_archive":                   {example},
		"weave_field_overrides_archive":            {override, model, collection, field},
		"weave_fields_archive":                     {field},
		"weave_models_archive":                     {model},
		"weave_release_namespace_bindings_archive": {},
		"weave_override_refs_archive":              {override},
		"weave_project_actors_archive":             {},
		"weave_project_attributions_archive":       {},
		"weave_project_inheritance_archive":        {},
		"weave_project_ontology_versions_archive":  {},
		"weave_projects_archive":                   {},
		"weave_vocabularies_archive":               {vocabulary},
		vocabularyEntriesSection:                   {vocabulary, "vocabulary_entry"},
	}
}

func discoverSection(section string, selection projectsnapshot.Selection) (bool, error) {
	kinds, ok := discoveryKinds()[section]
	if !ok {
		return false, fmt.Errorf("unregistered discovery section %s", section)
	}
	for _, kind := range kinds {
		if len(selection[kind]) > 0 {
			return true, nil
		}
	}
	return false, nil
}

func nextSelection(current, visited projectsnapshot.Selection) projectsnapshot.Selection {
	out := projectsnapshot.Selection{}
	for kind, ids := range current {
		for _, id := range ids {
			if !slices.Contains(visited[kind], id) {
				out[kind] = append(out[kind], id)
			}
		}
		visited[kind] = slices.Clone(ids)
	}
	return out
}

// A returned child row has already supplied its references. Its own identity
// need not trigger discovery again when it selects no other section. Derive that
// condition from the shared registry so adding another consumer disables the
// shortcut automatically; final materialization still uses the full selection.
func discoveredChildren(content projectsnapshot.Content, visited projectsnapshot.Selection) error {
	const kind = "vocabulary_entry"
	for section, kinds := range discoveryKinds() {
		if section != vocabularyEntriesSection && slices.Contains(kinds, kind) {
			return nil
		}
	}
	for _, row := range content.VocabularyEntries {
		if _, err := visited.Include(kind, row.ID); err != nil {
			return err
		}
	}
	return nil
}

func checkDiscovered(content projectsnapshot.Content, selection projectsnapshot.Selection) error {
	requests, err := projectsnapshot.RequiredEntities(content)
	if err != nil {
		return err
	}
	for kind, ids := range requests {
		for _, id := range ids {
			if !slices.Contains(selection[kind], id) {
				return fmt.Errorf("materialized %s %s escaped discovered closure", kind, id)
			}
		}
	}
	return nil
}
