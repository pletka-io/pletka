package example

import (
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
)

// buildExampleFormSchema composes one form shape; it never grants write capabilities.
func buildExampleFormSchema(resolver *slotResolver, exampleID, modelID string, name domain.Translations, record *ExampleRecord, lang string, languages []formschema.LanguageInfo) (*ExampleFormSchema, error) {
	if languages == nil {
		languages = []formschema.LanguageInfo{}
	}
	view := resolver.view
	var values []domain.ExampleValue
	var issues []domain.ExampleIssue
	if record != nil {
		values, issues = record.Values, record.Validation.Issues
	}
	// A value whose field moved to a different group since it was saved does
	// not resolve and stays out of the form, as it stays out of Get's counts.
	slots, err := resolver.resolveAll(values)
	if err != nil {
		return nil, err
	}
	issuesByKey, groupIssues, topIssues := routeIssues(issues)
	builder := newFormBuilder(resolver, values, slots, issuesByKey)
	present := formGroupPresence(values, slots)
	sections := make([]ExampleFormSection, 0, len(view.Categories))
	for _, cat := range view.Categories {
		section := ExampleFormSection{
			ID:             cat.ID,
			Label:          cat.Name,
			CanonicalOrder: cat.Position,
		}
		for _, coll := range cat.Collections {
			entries, err := buildGroupEntries(builder, coll, groupIssues, present[coll.ID])
			if err != nil {
				return nil, err
			}
			section.Groups = append(section.Groups, entries...)
		}
		sections = append(sections, section)
	}
	topIssues = append(topIssues, builder.unplacedIssues(issues)...)
	schema := &ExampleFormSchema{
		Kind:      "example-form",
		ExampleID: exampleID,
		Target: ExampleFormTarget{
			EntityType: string(domain.ExampleEntityTypeModel),
			EntityID:   modelID,
			Name:       name,
		},
		Sections: sections,
		Issues:   topIssues,
		UI: formschema.SchemaUI{
			SubmitLabel:    domain.Translations{"en": submitLabel(exampleID)},
			CancelLabel:    domain.Translations{"en": "Cancel"},
			SuccessMessage: domain.Translations{"en": successMessage(exampleID)},
			Languages:      languages,
			PrimaryLang:    lang,
		},
	}
	return schema, nil
}
