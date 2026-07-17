package ontology

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
)

func TestBuildFamilyEntityListSchema(t *testing.T) {
	schema := BuildFamilyEntityListSchema("en", nil)
	if schema == nil {
		t.Fatal("nil schema")
	}
	if schema.DataURL != "/admin/ontologies/families/data" {
		t.Fatalf("DataURL=%q want /admin/ontologies/families/data", schema.DataURL)
	}
	if schema.DetailURLTemplate != "/admin/ontologies/families/{id}/page" {
		t.Fatalf("DetailURLTemplate=%q", schema.DetailURLTemplate)
	}
	if schema.Search == nil || schema.Search.ParamName != "search" {
		t.Fatalf("unexpected search config: %+v", schema.Search)
	}
	if schema.Capabilities == nil || schema.Capabilities.Create == nil || schema.Capabilities.Delete == nil {
		t.Fatalf("missing capabilities: %+v", schema.Capabilities)
	}
	if len(schema.RowActions) != 2 {
		t.Fatalf("row action count=%d want 2", len(schema.RowActions))
	}
}

func TestBuildFamilyFormSchema_ParentFamilyOptions(t *testing.T) {
	parent := "family-1"
	families := []*domain.OntologyFamily{
		{ID: "family-1", Slug: "crm", Name: "CIDOC CRM"},
		{ID: "family-2", Slug: "sari", Name: "SARI", ParentFamilyID: &parent},
	}

	// Edit mode: the family being edited is excluded; current parent
	// is preselected as the field value.
	editSchema := BuildFamilyFormSchema(formschema.ModeEdit, families[1], families, "en", nil)
	identity := findSection(t, editSchema, "identity")
	parentField := findField(t, identity, "parent_family_id")
	if parentField.Value != "family-1" {
		t.Fatalf("parent_family_id value=%v want family-1", parentField.Value)
	}
	if len(parentField.Options) != 2 {
		t.Fatalf("option count=%d want 2 (none + family-1)", len(parentField.Options))
	}
	if parentField.Options[0].Value != "" {
		t.Fatalf("first option value=%q want empty (root family sentinel)", parentField.Options[0].Value)
	}
	for _, opt := range parentField.Options {
		if opt.Value == "family-2" {
			t.Fatalf("self-parenting option leaked: %+v", opt)
		}
	}

	// Create mode: no exclusion, no preselected value.
	createSchema := BuildFamilyFormSchema(formschema.ModeCreate, nil, families, "en", nil)
	createParent := findField(t, findSection(t, createSchema, "identity"), "parent_family_id")
	if createParent.Value != nil {
		t.Fatalf("create-mode parent_family_id value=%v want nil", createParent.Value)
	}
	if len(createParent.Options) != 3 {
		t.Fatalf("create-mode option count=%d want 3 (none + 2 families)", len(createParent.Options))
	}
}

func TestBuildVersionFormSchema_MetadataEditingFields(t *testing.T) {
	version := &domain.OntologyVersion{
		ID:                     "ver-1",
		VersionString:          "2.1.1",
		CompatibleBaseVersions: []string{"7.1.3", "7.1.2"},
		ImportedOntologies:     []string{"crm:7.1.3"},
		VersionInfo:            domain.Translations{"en": "Imported from upstream release"},
		OriginalFilename:       "CRMarchaeo_v2.1.1.rdf",
		FileMD5:                "abc123",
	}

	schema := BuildVersionFormSchema(version, "en", nil)
	identity := findSection(t, schema, "identity")
	if got := findField(t, identity, "version_info").Value.(domain.Translations)["en"]; got != "Imported from upstream release" {
		t.Fatalf("version_info=%q", got)
	}

	compatibility := findSection(t, schema, "compatibility")
	if got := findField(t, compatibility, "compatible_base_versions_text").Value; got != "7.1.3\n7.1.2" {
		t.Fatalf("compatible_base_versions_text=%q", got)
	}
	if got := findField(t, compatibility, "imported_ontologies_text").Value; got != "crm:7.1.3" {
		t.Fatalf("imported_ontologies_text=%q", got)
	}

	source := findSection(t, schema, "source")
	if !findField(t, source, "original_filename").Readonly {
		t.Fatal("original_filename should be readonly")
	}
	if !source.Collapsed {
		t.Fatal("source section should be collapsed")
	}
}

func findSection(t *testing.T, schema *formschema.FormSchema, id string) formschema.Section {
	t.Helper()
	for _, s := range schema.Sections {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("section %q not found", id)
	return formschema.Section{}
}

func findField(t *testing.T, section formschema.Section, name string) formschema.FieldDef {
	t.Helper()
	for _, f := range section.Fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("field %q not found in section %q", name, section.ID)
	return formschema.FieldDef{}
}

func TestBuildOntologyEntityListSchema(t *testing.T) {
	familyOptions := []formschema.FilterOption{
		{Value: "family-1", Label: domain.Translations{"en": "CRM Family"}},
	}
	schema := BuildOntologyEntityListSchema("en", nil, familyOptions)
	if schema == nil {
		t.Fatal("nil schema")
	}
	if schema.DataURL != "/admin/ontologies/data" {
		t.Fatalf("DataURL=%q want /admin/ontologies/data", schema.DataURL)
	}
	if schema.DetailURLTemplate != "/admin/ontologies/{id}/page" {
		t.Fatalf("DetailURLTemplate=%q", schema.DetailURLTemplate)
	}
	if len(schema.Filters) != 2 {
		t.Fatalf("filter count=%d want 2", len(schema.Filters))
	}
	if got := len(schema.Filters[1].Options); got != 1 {
		t.Fatalf("family filter options=%d want 1", got)
	}
	if schema.Capabilities == nil || schema.Capabilities.Edit == nil || schema.Capabilities.Delete == nil {
		t.Fatalf("missing capabilities: %+v", schema.Capabilities)
	}
}
