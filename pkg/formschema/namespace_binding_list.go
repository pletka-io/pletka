package formschema

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildNamespaceBindingListSchema constructs the list schema for the
// namespace-bindings pane. The pane mixes user-owned and system-owned
// bindings; system rows are marked read-only via the PerRowReadonlyField
// so ListManager hides their row actions.
//
// Columns: prefix (primary), namespace (secondary), weight (badge), and
// source (badge). Source values are mapped to the shared Badge tones
// (primary|neutral|warning) at the data layer.
func BuildNamespaceBindingListSchema(projectID string, lang string, languages []LanguageInfo) *ListSchema {
	pid := projectID
	return &ListSchema{
		EntityType: "namespace-binding",
		Title:      i18n.L("namespace_binding.list.title", "Namespace bindings"),
		EmptyState: &EmptyState{
			Icon:    "link",
			Title:   i18n.L("namespace_binding.list.empty_title", "No namespace bindings"),
			Message: i18n.L("namespace_binding.list.empty_message", "Add a prefix-to-namespace binding to get started."),
		},
		DataURL:             fmt.Sprintf("/projects/%s/namespace-bindings", pid),
		PerRowReadonlyField: "_readonly",
		Caps: Capabilities{
			Create: &CreateCap{
				Label:         i18n.L("namespace_binding.list.add", "Add binding"),
				FormSchemaURL: fmt.Sprintf("/projects/%s/form-schema/namespace-binding?mode=create", pid),
			},
			Edit: &EditCap{
				FormSchemaURLTemplate: fmt.Sprintf("/projects/%s/form-schema/namespace-binding?mode=edit&entity_id={id}", pid),
			},
			Delete: &DeleteCap{
				URLTemplate: fmt.Sprintf("/projects/%s/namespace-bindings/{id}", pid),
			},
		},
		Columns: []Column{
			{
				Key:     "prefix",
				Label:   i18n.L("namespace_binding.list.prefix", "Prefix"),
				Type:    "text",
				Primary: true,
			},
			{
				Key:       "namespace",
				Label:     i18n.L("namespace_binding.list.namespace", "Namespace"),
				Type:      "text",
				Secondary: true,
			},
			{
				Key:        "weight",
				Label:      i18n.L("namespace_binding.list.weight", "Weight"),
				Type:       "badge",
				BadgeStyle: "gray",
			},
			{
				Key:   "source",
				Label: i18n.L("namespace_binding.list.source", "Source"),
				Type:  "text_badge",
				BadgeStyleByValue: map[string]string{
					"system":   "gray",
					"user":     "blue",
					"ontology": "purple",
					"manifest": "green",
				},
			},
		},
		RowActions: []RowAction{
			{ID: "edit", Icon: "pencil", Label: i18n.L("common.edit", "Edit")},
			{ID: "delete", Icon: "trash", Label: i18n.L("common.remove", "Remove"), Style: "danger"},
		},
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}
