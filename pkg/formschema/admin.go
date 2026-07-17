package formschema

import (
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// AdminSchema describes the global admin dashboard shell. It is the global
// equivalent of project-scoped SettingsSchema: a capability-filtered list of
// sections the frontend mounts using generic schema-driven renderers.
type AdminSchema struct {
	Title    domain.Localizable `json:"title"`
	Sections []AdminSection     `json:"sections"`
	Warnings []AdminWarning     `json:"warnings,omitempty"`
}

type AdminSection struct {
	ID          string             `json:"id"`
	Label       domain.Localizable `json:"label"`
	Icon        string             `json:"icon"`
	Kind        string             `json:"kind,omitempty"` // form, list, entity-list, composite, external, git-restore
	SchemaURL   string             `json:"schema_url,omitempty"`
	Href        string             `json:"href,omitempty"`
	Placeholder bool               `json:"placeholder,omitempty"`
}

type AdminWarning struct {
	ID          string             `json:"id"`
	Severity    string             `json:"severity"` // info, warning, error
	Message     domain.Localizable `json:"message"`
	ActionHref  string             `json:"action_href,omitempty"`
	ActionLabel domain.Localizable `json:"action_label,omitempty"`
}

// BuildAdminSchema returns the global superadmin dashboard surface. The shell
// currently exposes one real slice-owned section (Ontologies) plus a few
// placeholders that make the intended extension model visible without
// overcommitting the first implementation.
func BuildAdminSchema(snap *auth.AuthSnapshot, extraSections ...AdminSection) *AdminSchema {
	schema := &AdminSchema{
		Title: i18n.L("admin.shell.title", "Admin"),
	}

	if snap == nil || !snap.IsSuperAdmin {
		return schema
	}

	schema.Sections = []AdminSection{
		{
			ID:        "users",
			Label:     i18n.L("admin.users.title", "Users"),
			Icon:      "users",
			Kind:      "entity-list",
			SchemaURL: "/admin/users/entity-list-schema",
		},
		{
			ID:        "institutions",
			Label:     i18n.L("admin.institutions.title", "Institutions"),
			Icon:      "building-library",
			Kind:      "entity-list",
			SchemaURL: "/admin/institutions/entity-list-schema",
		},
		{
			ID:        "ontology-families",
			Label:     i18n.L("ontology_admin.family.title", "Ontology Families"),
			Icon:      "folder",
			Kind:      "entity-list",
			SchemaURL: "/admin/ontologies/families/entity-list-schema",
		},
		{
			ID:        "ontologies",
			Label:     i18n.L("ontology_admin.ontology.title", "Ontologies"),
			Icon:      "academic-cap",
			Kind:      "entity-list",
			SchemaURL: "/admin/ontologies/entity-list-schema",
		},
		{
			ID:        "global-namespaces",
			Label:     i18n.L("namespace_binding.list.global_entity_title", "Global Namespaces"),
			Icon:      "at-symbol",
			Kind:      "entity-list",
			SchemaURL: "/admin/namespaces/entity-list-schema",
		},
		{
			ID:        "vocabularies",
			Label:     i18n.L("admin.vocabularies.title", "Vocabularies"),
			Icon:      "book-open",
			Kind:      "entity-list",
			SchemaURL: "/admin/vocabularies/entity-list-schema",
		},
		{
			ID:    "git-restore",
			Label: i18n.L("admin.git_restore.title", "Git Restore"),
			Icon:  "arrow-path",
			Kind:  "git-restore",
		},
		{
			ID:    "materialization",
			Label: i18n.L("admin.materialization.title", "Git Materialization"),
			Icon:  "arrow-path-rounded-square",
			Kind:  "materialization",
		},
		{
			ID:          "jsonld-contexts",
			Label:       i18n.L("admin.shell.jsonld_contexts", "JSON-LD Contexts"),
			Icon:        "code-bracket",
			Kind:        "list",
			Placeholder: true,
		},
		{
			ID:          "generators",
			Label:       i18n.L("admin.shell.generators", "Generator Configurations"),
			Icon:        "cog-6-tooth",
			Kind:        "composite",
			Placeholder: true,
		},
	}
	schema.Sections = append(schema.Sections, extraSections...)

	return schema
}
