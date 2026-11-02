package projectread

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/weave/collection"
	"github.com/pletka-io/pletka/pkg/weave/example"
	"github.com/pletka-io/pletka/pkg/weave/override"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

func loadContent(ctx context.Context, s *readsession.Session, b *budget) (projectsnapshot.Content, map[string]int64, error) {
	c := projectsnapshot.Content{}
	counts := map[string]int64{}
	legacy, err := s.ContentRows()
	if err != nil {
		return c, nil, err
	}
	collections, err := collection.BindReader(s)
	if err != nil {
		return c, nil, err
	}
	overrides, err := override.BindReader(s)
	if err != nil {
		return c, nil, err
	}
	concepts, err := vocabulary.BindContentReader(s)
	if err != nil {
		return c, nil, err
	}
	examples, err := example.BindContentReader(s)
	if err != nil {
		return c, nil, err
	}
	loaders := []func() error{
		func() error {
			return loadSection(ctx, b, "weave_adoptions_archive", legacy.ContentAdoptions, &c.Adoptions, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_categories_archive", legacy.ContentCategories, &c.Categories, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_collection_placements_archive", overrides.ContentPlacements, &c.Placements, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_collections_archive", collections.ContentCollections, &c.Collections, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_concept_broader_archive", concepts.ContentBroader, &c.Broader, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_concept_list_entries_archive", concepts.ContentConceptEntries, &c.ConceptEntries, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_concept_lists_archive", concepts.ContentConceptLists, &c.ConceptLists, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_entity_forks_archive", legacy.ContentForks, &c.Forks, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_example_values_archive", examples.ContentExampleValues, &c.ExampleValues, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_examples_archive", examples.ContentExamples, &c.Examples, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_field_overrides_archive", overrides.ContentOverrides, &c.Overrides, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_fields_archive", legacy.ContentFields, &c.Fields, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_models_archive", legacy.ContentModels, &c.Models, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_release_namespace_bindings_archive", legacy.ContentNamespaces, &c.Namespaces, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_override_refs_archive", overrides.ContentOverrideRefs, &c.OverrideRefs, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_project_actors_archive", legacy.ContentProjectActors, &c.ProjectActors, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_project_attributions_archive", legacy.ContentAttributions, &c.Attributions, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_project_inheritance_archive", legacy.ContentInheritance, &c.Inheritance, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_project_ontology_versions_archive", legacy.ContentOntologyLinks, &c.OntologyLinks, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_projects_archive", legacy.ContentProjects, &c.Projects, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_vocabularies_archive", concepts.ContentVocabularies, &c.Vocabularies, counts)
		},
		func() error {
			return loadSection(ctx, b, "weave_vocabulary_entries_archive", concepts.ContentVocabularyEntries, &c.VocabularyEntries, counts)
		},
	}
	for _, load := range loaders {
		if err := load(); err != nil {
			return projectsnapshot.Content{}, nil, err
		}
	}
	return c, counts, nil
}
func loadSection[T any](ctx context.Context, b *budget, section string, read func(context.Context, readsession.Page) ([]T, error), destination *[]T, counts map[string]int64) error {
	rows, err := pages(ctx, b, read)
	if err != nil {
		return fmt.Errorf("load %s: %w", section, err)
	}
	*destination = rows
	counts[section] = int64(len(rows))
	return nil
}
