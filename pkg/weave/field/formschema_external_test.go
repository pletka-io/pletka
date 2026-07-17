package field_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave/field"
)

func TestBuildFieldSchema_Create(t *testing.T) {
	langs := []formschema.LanguageInfo{{Code: "en", Name: "English"}, {Code: "nl", Name: "Nederlands", Flag: "🇳🇱"}}
	schema := field.BuildFormSchema(formschema.ModeCreate, nil, "01HPROJ", "en", langs)

	if schema.EntityType != "field" {
		t.Errorf("EntityType = %q, want 'field'", schema.EntityType)
	}
	if schema.Mode != formschema.ModeCreate {
		t.Errorf("Mode = %q, want 'create'", schema.Mode)
	}
	if schema.Endpoint == nil {
		t.Fatal("Endpoint should not be nil for create mode")
	}
	if schema.Endpoint.Method != "POST" {
		t.Errorf("Endpoint.Method = %q, want 'POST'", schema.Endpoint.Method)
	}
	if !strings.Contains(schema.Endpoint.URL, "01HPROJ") {
		t.Errorf("Endpoint.URL %q should contain projectID", schema.Endpoint.URL)
	}

	// Should have identity, ontology, and override sections.
	if len(schema.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(schema.Sections))
	}

	sectionIDs := make(map[string]bool)
	for _, sec := range schema.Sections {
		sectionIDs[sec.ID] = true
	}
	for _, expected := range []string{"identity", "ontology", "override"} {
		if !sectionIDs[expected] {
			t.Errorf("missing section %q", expected)
		}
	}

	// Identity section should have ui_name, description, system_name
	var identitySection *formschema.Section
	for i := range schema.Sections {
		if schema.Sections[i].ID == "identity" {
			identitySection = &schema.Sections[i]
			break
		}
	}
	if identitySection == nil {
		t.Fatal("identity section not found")
	}

	fieldNames := make(map[string]bool)
	for _, f := range identitySection.Fields {
		fieldNames[f.Name] = true
	}
	for _, expected := range []string{"ui_name", "description", "system_name"} {
		if !fieldNames[expected] {
			t.Errorf("missing field %q in identity section", expected)
		}
	}

	// system_name should be immutable_after_create and derived from ui_name.en
	for _, f := range identitySection.Fields {
		if f.Name == "system_name" {
			if !f.ImmutableAfterCreate {
				t.Error("system_name should be immutable_after_create")
			}
			if f.DerivedFrom != "ui_name.en" {
				t.Errorf("system_name.DerivedFrom = %q, want 'ui_name.en'", f.DerivedFrom)
			}
		}
	}
}

func TestBuildFieldSchema_Edit(t *testing.T) {
	existing := &formschema.ComposedFieldInput{
		Field: formschema.FieldInput{
			ID:            "01HFIELD",
			SystemName:    "birth_date",
			UIName:        domain.Translations{"en": "Birth Date", "nl": "Geboortedatum"},
			Description:   domain.Translations{"en": "Date of birth"},
			OntologyScope: domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E21_Person"},
			PathElements: []domain.PathElement{
				{Type: "property", Prefix: "crm", LocalName: "P98i_was_born", Position: 0},
				{Type: "class", Prefix: "crm", LocalName: "E67_Birth", Position: 1},
				{Type: "property", Prefix: "crm", LocalName: "P4_has_time-span", Position: 2},
			},
			ExpectedValueType: "Date",
			Examples:          "1900-01-01",
		},
		Override: formschema.FieldOverrideInput{
			ExpectedValueType:     "Date",
			CategoryID:            "CAT.1",
			SetValue:              "1900-01-01",
			ExpectedModelIDs:      []string{"MODEL.1"},
			ExpectedCollectionIDs: []string{"COLL.1"},
		},
	}

	langs := []formschema.LanguageInfo{{Code: "en", Name: "English"}}
	schema := field.BuildFormSchema(formschema.ModeEdit, existing, "01HPROJ", "en", langs)

	if schema.Mode != formschema.ModeEdit {
		t.Errorf("Mode = %q, want 'edit'", schema.Mode)
	}
	if schema.Endpoint == nil || schema.Endpoint.Method != "PUT" {
		t.Error("edit mode should have PUT endpoint")
	}
	if !strings.Contains(schema.Endpoint.URL, "01HFIELD") {
		t.Errorf("Endpoint.URL %q should contain field ID", schema.Endpoint.URL)
	}

	// system_name should be readonly in edit mode
	for _, sec := range schema.Sections {
		for _, f := range sec.Fields {
			if f.Name == "system_name" && !f.Readonly {
				t.Error("system_name should be readonly in edit mode")
			}
		}
	}

	// Existing values should be populated
	for _, sec := range schema.Sections {
		for _, f := range sec.Fields {
			if f.Name == "ui_name" {
				if f.Value == nil {
					t.Error("ui_name should have existing value populated")
				}
			}
			if f.Name == "ontology_scope" {
				if f.Value == nil {
					t.Error("ontology_scope should have existing value populated")
				}
			}
			if f.Name == "expected_value_type" {
				if f.Value != "Date" {
					t.Errorf("expected_value_type value = %v, want 'Date'", f.Value)
				}
			}
		}
	}
}

func TestBuildFieldSchema_OntologySection(t *testing.T) {
	langs := []formschema.LanguageInfo{{Code: "en", Name: "English"}}
	schema := field.BuildFormSchema(formschema.ModeCreate, nil, "01HPROJ", "en", langs)

	var ontologySection *formschema.Section
	for i := range schema.Sections {
		if schema.Sections[i].ID == "ontology" {
			ontologySection = &schema.Sections[i]
			break
		}
	}
	if ontologySection == nil {
		t.Fatal("ontology section not found")
	}

	widgetTypes := make(map[string]string)
	for _, f := range ontologySection.Fields {
		widgetTypes[f.Name] = f.Widget
	}

	for _, name := range []string{"ontology_scope", "ontology_path"} {
		widget, ok := widgetTypes[name]
		if !ok {
			t.Errorf("missing field %q in ontology section", name)
			continue
		}
		if widget != formschema.WidgetOntologyPath {
			t.Errorf("field %q widget = %q, want %q", name, widget, formschema.WidgetOntologyPath)
		}
	}
}

func TestBuildFieldSchema_CreateMode(t *testing.T) {
	languages := []formschema.LanguageInfo{
		{Code: "en", Name: "English"},
		{Code: "nl", Name: "Nederlands"},
	}

	schema := field.BuildFormSchema(formschema.ModeCreate, nil, "PROJECT123", "en", languages)

	if schema.Mode != formschema.ModeCreate {
		t.Errorf("mode = %q, want %q", schema.Mode, formschema.ModeCreate)
	}
	if schema.Endpoint == nil || schema.Endpoint.Method != "POST" {
		t.Fatalf("endpoint method not POST: %+v", schema.Endpoint)
	}

	if len(schema.Sections) != 3 {
		t.Fatalf("sections = %d, want 3", len(schema.Sections))
	}

	// Identity
	identityNames := fieldNames(schema.Sections[0].Fields)
	wantIdentity := []string{"ui_name", "description", "system_name"}
	if diff := cmp.Diff(wantIdentity, identityNames); diff != "" {
		t.Errorf("identity fields (-want +got):\n%s", diff)
	}

	// Ontology — scope (root class) + path (chain).
	ontologyNames := fieldNames(schema.Sections[1].Fields)
	wantOntology := []string{"ontology_scope", "ontology_path"}
	if diff := cmp.Diff(wantOntology, ontologyNames); diff != "" {
		t.Errorf("ontology fields (-want +got):\n%s", diff)
	}

	override := schema.Sections[2]
	overrideNames := fieldNames(override.Fields)
	wantOverride := []string{"expected_value_type", "expected_resource_models", "expected_collection_models", "set_value", "category_id"}
	if diff := cmp.Diff(wantOverride, overrideNames); diff != "" {
		t.Errorf("override fields (-want +got):\n%s", diff)
	}

	// file/bitstream in expected_value_type options
	evtField := findField(override.Fields, "expected_value_type")
	if evtField == nil {
		t.Fatal("expected_value_type not found")
	}
	found := false
	for _, opt := range evtField.Options {
		if opt.Value == "file/bitstream" {
			found = true
			break
		}
	}
	if !found {
		t.Error("file/bitstream option missing from expected_value_type")
	}
}

func TestBuildFieldOverrideSchema_CreateMode(t *testing.T) {
	languages := []formschema.LanguageInfo{{Code: "en", Name: "English"}}
	schema := formschema.BuildFieldOverrideSchema(formschema.ModeCreate, nil, "PROJECT123", languages, "en")

	if schema.EntityType != "field-override" {
		t.Fatalf("entity_type=%q", schema.EntityType)
	}
	if len(schema.Sections) != 1 {
		t.Fatalf("sections=%d want 1", len(schema.Sections))
	}

	fields := schema.Sections[0].Fields
	got := fieldNames(fields)
	want := []string{"expected_value_type", "expected_resource_models", "expected_collection_models", "set_value", "category_id"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("override fields (-want +got):\n%s", diff)
	}
}

func fieldNames(fields []formschema.FieldDef) []string {
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
	}
	return names
}

func findField(fields []formschema.FieldDef, name string) *formschema.FieldDef {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
	}
	return nil
}

func ontologyFields(t *testing.T, schema *formschema.FormSchema) []formschema.FieldDef {
	t.Helper()
	for _, sec := range schema.Sections {
		if sec.ID == "ontology" {
			return sec.Fields
		}
	}
	t.Fatal("no ontology section")
	return nil
}

func TestBuildFieldSchema_SubfieldPathsReadOnly(t *testing.T) {
	langs := []formschema.LanguageInfo{{Code: "en", Name: "English"}}
	existing := &formschema.ComposedFieldInput{
		Field: formschema.FieldInput{
			ID:            "SRD1F.5",
			SystemName:    "name",
			OntologyScope: domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E21_Person", URI: "crm:E21_Person"},
			PathElements: []domain.PathElement{
				{Type: "property", Prefix: "crm", LocalName: "P1_is_identified_by", URI: "crm:P1_is_identified_by"},
				{Type: "literal", Prefix: "rdf", LocalName: "literal", URI: "rdf:literal"},
			},
			SubfieldPaths: []domain.SubfieldPath{{
				Source:            "legacy-br-split",
				ExpectedValueType: "crm:E55_Type",
				PathElements: []domain.PathElement{
					{Type: "property", Prefix: "crm", LocalName: "P2_has_type", URI: "crm:P2_has_type"},
					{Type: "class", Prefix: "crm", LocalName: "E55_Type", URI: "crm:E55_Type"},
				},
			}},
		},
	}

	// Edit mode: subfield_paths field present, read-only, correct widget + value.
	editSchema := field.BuildFormSchema(formschema.ModeEdit, existing, "SRD1", "en", langs)
	sf := findField(ontologyFields(t, editSchema), "subfield_paths")
	if sf == nil {
		t.Fatalf("edit mode missing subfield_paths field; got %v", fieldNames(ontologyFields(t, editSchema)))
	}
	if sf.Widget != formschema.WidgetSubfieldPaths {
		t.Errorf("widget = %q, want %q", sf.Widget, formschema.WidgetSubfieldPaths)
	}
	if !sf.Readonly {
		t.Error("subfield_paths must be read-only")
	}
	if got, ok := sf.Value.([]domain.SubfieldPath); !ok || len(got) != 1 {
		t.Errorf("value = %#v, want 1 SubfieldPath", sf.Value)
	}

	// No subfields → field omitted.
	existing.Field.SubfieldPaths = nil
	if sf := findField(ontologyFields(t, field.BuildFormSchema(formschema.ModeEdit, existing, "SRD1", "en", langs)), "subfield_paths"); sf != nil {
		t.Error("subfield_paths field should be omitted when none present")
	}
}
