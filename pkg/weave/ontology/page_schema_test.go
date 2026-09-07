package ontology

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
)

func TestBuildOntologyDetailPageSchema_AdminActionsAreGated(t *testing.T) {
	model := &OntologyDetailPageModel{
		Name:         "CIDOC CRM",
		Prefix:       "crm",
		Namespace:    "http://www.cidoc-crm.org/cidoc-crm/",
		OntologyType: "Base",
		Versions: []VersionPageCardModel{
			{
				ID:            "ver-1",
				OntologyID:    "ont-1",
				VersionString: "7.1.3",
				IsActive:      true,
				VersionURL:    "/ontologies/crm/7.1.3",
				ClassesURL:    "/ontologies/crm/7.1.3?tab=classes",
				PropertiesURL: "/ontologies/crm/7.1.3/properties",
			},
		},
	}

	public := BuildOntologyDetailPageSchema(model, "en", nil, false)
	if len(public.Actions) != 0 {
		t.Fatalf("public actions=%d want 0", len(public.Actions))
	}

	admin := BuildOntologyDetailPageSchema(model, "en", nil, true)
	if len(admin.Actions) == 0 {
		t.Fatal("admin actions missing")
	}
	if admin.Kind != "ontology-detail" {
		t.Fatalf("kind=%q want ontology-detail", admin.Kind)
	}
	if got := len(admin.Sections); got == 0 {
		t.Fatal("sections missing")
	}
}

func TestBuildVersionDetailPageSchema_SetActiveOnlyWhenInactive(t *testing.T) {
	model := &VersionDetailPageModel{
		Name:          "CIDOC CRM",
		Prefix:        "crm",
		Namespace:     "http://www.cidoc-crm.org/cidoc-crm/",
		VersionString: "7.1.3",
		ActiveTab:     "overview",
		Version: VersionPageCardModel{
			ID:            "ver-1",
			OntologyID:    "ont-1",
			VersionString: "7.1.3",
			IsActive:      false,
			VersionURL:    "/ontologies/crm/7.1.3",
			ClassesURL:    "/ontologies/crm/7.1.3?tab=classes",
			PropertiesURL: "/ontologies/crm/7.1.3/properties",
		},
	}

	schema := BuildVersionDetailPageSchema(model, "en", nil, true)
	if !hasOntologyPageAction(schema.Actions, "set-active") {
		t.Fatal("set-active action missing for inactive admin version")
	}

	model.Version.IsActive = true
	schema = BuildVersionDetailPageSchema(model, "en", nil, true)
	if hasOntologyPageAction(schema.Actions, "set-active") {
		t.Fatal("set-active action present for active version")
	}
}

func TestBuildVersionDetailPageSchema_IncludesLifecycleImportCompatibility(t *testing.T) {
	model := &VersionDetailPageModel{
		Name:                "CIDOC CRM",
		Prefix:              "crm",
		Namespace:           "http://www.cidoc-crm.org/cidoc-crm/",
		VersionString:       "7.1.3",
		ActiveTab:           "overview",
		ActiveVersionString: "7.1.3",
		ProjectUsages: []VersionProjectUsageModel{
			{ProjectID: "LA", ProjectName: "Linked Archive", ProjectURL: "/projects/LA", AddedAt: "2026-05-16", IsPrimary: true},
		},
		Version: VersionPageCardModel{
			ID:                     "ver-1",
			OntologyID:             "ont-1",
			VersionString:          "7.1.3",
			IsActive:               false,
			ProjectCount:           0,
			CompatibleBaseVersions: []string{"7.1.2"},
			ImportedOntologies:     []string{"http://example.org/imported"},
			OntologyURI:            "http://example.org/ontology",
			VersionIRI:             "http://example.org/ontology/7.1.3",
			OriginalFilename:       "crm.ttl",
			FileSize:               128,
			FileMD5:                "abc123",
			ParsedAt:               "2026-05-15 10:00",
			VersionURL:             "/ontologies/crm/7.1.3",
			ClassesURL:             "/ontologies/crm/7.1.3?tab=classes",
			PropertiesURL:          "/ontologies/crm/7.1.3/properties",
		},
	}

	schema := BuildVersionDetailPageSchema(model, "en", nil, true)
	for _, id := range []string{"lifecycle", "identity", "import-metadata", "compatibility", "project-adoption", "import-history", "usage-impact"} {
		if !hasOntologyPageSection(schema.Sections, id) {
			t.Fatalf("section %q missing", id)
		}
	}
	if action := findOntologyPageAction(schema.Actions, "set-active"); action == nil {
		t.Fatal("set-active action missing")
	} else if action.Confirm == nil {
		t.Fatal("set-active confirmation missing")
	}
	if action := findOntologyPageAction(schema.Actions, "delete"); action == nil {
		t.Fatal("delete action missing for unused inactive admin version")
	} else {
		if action.Method != "DELETE" {
			t.Fatalf("delete method=%q want DELETE", action.Method)
		}
		if action.Confirm == nil {
			t.Fatal("delete confirmation missing")
		}
		if action.SuccessHref == "" {
			t.Fatal("delete success redirect missing")
		}
	}
	if !hasOntologyPageAction(schema.Actions, "import") {
		t.Fatal("import action missing")
	}

	model.Version.ProjectCount = 1
	schema = BuildVersionDetailPageSchema(model, "en", nil, true)
	if hasOntologyPageAction(schema.Actions, "delete") {
		t.Fatal("delete action present for in-use version")
	}
}

func TestBuildFamilyLandingPageSchemaIncludesNarrowWidgets(t *testing.T) {
	model := &FamilyLandingPageModel{
		FamilyCount:     1,
		RootFamilyCount: 1,
		OntologyCount:   2,
		Families: []FamilyPageCardModel{
			{ID: "fam-1", Name: "CIDOC CRM family", Slug: "cidoc-crm", URL: "/ontologies/families/cidoc-crm"},
		},
	}

	schema := BuildFamilyLandingPageSchema(model, "en", nil)
	if schema.Kind != "ontology-family-landing" {
		t.Fatalf("kind=%q want ontology-family-landing", schema.Kind)
	}
	if len(schema.Sections) != 2 {
		t.Fatalf("section count=%d want 2", len(schema.Sections))
	}
	if schema.Sections[0].Widget != "stats-strip" || schema.Sections[1].Widget != "card-list" {
		t.Fatalf("widgets=%q,%q want stats-strip,card-list", schema.Sections[0].Widget, schema.Sections[1].Widget)
	}
}

func TestBuildFamilyDetailPageSchema_AdminEditActionGated(t *testing.T) {
	model := &FamilyDetailPageModel{
		Family: &domain.OntologyFamily{ID: "fam-123", Slug: "cidoc-crm-family", Name: "CIDOC CRM Family"},
		Name:   "CIDOC CRM Family",
	}

	public := BuildFamilyDetailPageSchema(model, "en", nil)
	if hasOntologyPageAction(public.Actions, actionIDEdit) {
		t.Fatal("public family schema must not expose the edit action")
	}

	admin := BuildAdminFamilyDetailPageSchema(model, "en", nil)
	edit := findOntologyPageAction(admin.Actions, actionIDEdit)
	if edit == nil {
		t.Fatal("admin family schema missing edit action (F3c: super-admin lands on a read-only family page)")
	}
	if want := "/admin/ontologies/families/form-schema?mode=edit&entity_id=fam-123"; edit.FormSchemaURL != want {
		t.Fatalf("edit form url=%q want %q", edit.FormSchemaURL, want)
	}
}

func hasOntologyPageAction(actions []formschema.OntologyPageAction, id string) bool {
	return findOntologyPageAction(actions, id) != nil
}

func findOntologyPageAction(actions []formschema.OntologyPageAction, id string) *formschema.OntologyPageAction {
	for _, action := range actions {
		if action.ID == id {
			return &action
		}
	}
	return nil
}

func hasOntologyPageSection(sections []formschema.OntologyPageSection, id string) bool {
	for _, section := range sections {
		if section.ID == id {
			return true
		}
	}
	return false
}
