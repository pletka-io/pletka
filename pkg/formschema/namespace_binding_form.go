package formschema

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildNamespaceBindingCreateForm constructs the create form for a
// project-scoped namespace binding. Required fields: prefix and namespace.
// Weight defaults to 10 (lower weight sorts first when resolving prefixes).
func BuildNamespaceBindingCreateForm(projectID string, lang string, languages []LanguageInfo) *FormSchema {
	return &FormSchema{
		EntityType: "namespace-binding",
		Mode:       ModeCreate,
		Endpoint: &SchemaEndpoint{
			Method: "POST",
			URL:    fmt.Sprintf("/projects/%s/namespace-bindings", projectID),
		},
		UI: SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("namespace_binding.form.submit_create", "Add binding"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("namespace_binding.form.created", "Namespace binding created"),
		},
		Sections: []Section{
			{
				ID: "identity",
				Fields: []FieldDef{
					{
						Name:     "prefix",
						Widget:   WidgetText,
						Required: true,
						Label:    i18n.L("namespace_binding.form.prefix", "Prefix"),
						Help:     i18n.L("namespace_binding.form.prefix_help", "Short identifier used in curie-style references (e.g. crm)."),
					},
					{
						Name:     "namespace",
						Widget:   WidgetText,
						Required: true,
						Label:    i18n.L("namespace_binding.form.namespace", "Namespace"),
						Help:     i18n.L("namespace_binding.form.namespace_help", "Full namespace URI the prefix expands to."),
					},
					{
						Name:   "weight",
						Widget: WidgetNumber,
						Label:  i18n.L("namespace_binding.form.weight", "Weight"),
						Help:   i18n.L("namespace_binding.form.weight_help_create", "Lower weight resolves first. Defaults to 10."),
						Value:  10,
					},
				},
			},
		},
	}
}

// BuildNamespaceBindingEditForm constructs the edit form for an existing
// user-owned namespace binding. The three fields are all editable; system
// rows never reach this form because the list hides their row actions.
func BuildNamespaceBindingEditForm(projectID string, binding *domain.NamespaceBinding, lang string, languages []LanguageInfo) *FormSchema {
	var (
		prefix    string
		namespace string
		weight    int64
		id        string
	)
	if binding != nil {
		prefix = binding.Prefix
		namespace = binding.Namespace
		weight = binding.Weight
		id = binding.ID
	}

	return &FormSchema{
		EntityType: "namespace-binding",
		Mode:       ModeEdit,
		Endpoint: &SchemaEndpoint{
			Method: "PATCH",
			URL:    fmt.Sprintf("/projects/%s/namespace-bindings/%s", projectID, id),
		},
		UI: SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("forms.save_changes", "Save Changes"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("namespace_binding.form.updated", "Namespace binding updated"),
		},
		Sections: []Section{
			{
				ID: "identity",
				Fields: []FieldDef{
					{
						Name:     "prefix",
						Widget:   WidgetText,
						Required: true,
						Label:    i18n.L("namespace_binding.form.prefix", "Prefix"),
						Help:     i18n.L("namespace_binding.form.prefix_help", "Short identifier used in curie-style references (e.g. crm)."),
						Value:    prefix,
					},
					{
						Name:     "namespace",
						Widget:   WidgetText,
						Required: true,
						Label:    i18n.L("namespace_binding.form.namespace", "Namespace"),
						Help:     i18n.L("namespace_binding.form.namespace_help", "Full namespace URI the prefix expands to."),
						Value:    namespace,
					},
					{
						Name:   "weight",
						Widget: WidgetNumber,
						Label:  i18n.L("namespace_binding.form.weight", "Weight"),
						Help:   i18n.L("namespace_binding.form.weight_help", "Lower weight resolves first."),
						Value:  weight,
					},
				},
			},
		},
	}
}
