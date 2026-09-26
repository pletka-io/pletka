package settings

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
)

func snap(actorID string, roles map[string]string) *auth.AuthSnapshot {
	return &auth.AuthSnapshot{
		ActorID:         actorID,
		Roles:           roles,
		OwnedProjectIDs: map[string]struct{}{},
	}
}

func sectionIDs(ss []formschema.SettingsSection) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.ID)
	}
	return out
}

func assertSameSet(t *testing.T, got, want []string) {
	t.Helper()
	m := map[string]bool{}
	for _, s := range want {
		m[s] = true
	}
	for _, s := range got {
		if !m[s] {
			t.Errorf("unexpected section: %s", s)
		}
		delete(m, s)
	}
	for s := range m {
		t.Errorf("missing section: %s", s)
	}
}

func TestBuildSettingsSchema_UnderThreeIdentities(t *testing.T) {
	p := &domain.Project{
		Entity:     domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}},
		Visibility: "private",
	}
	res := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "private"}

	allSections := []string{"general", "about", "ontology", "categories", "members", "attributions", "namespace-bindings", "vocabularies", "autocomplete", "integrations"}

	cases := []struct {
		name    string
		snap    *auth.AuthSnapshot
		wantIDs []string
	}{
		{
			name:    "anonymous",
			snap:    &auth.AuthSnapshot{IsAnonymous: true},
			wantIDs: []string{},
		},
		{
			name:    "contributor",
			snap:    snap("u1", map[string]string{"project:LA": "contributor"}),
			wantIDs: []string{"categories", "attributions"},
		},
		{
			name:    "maintainer",
			snap:    snap("u2", map[string]string{"project:LA": "maintainer"}),
			wantIDs: allSections,
		},
		{
			name:    "owner",
			snap:    snap("u3", map[string]string{"project:LA": "owner"}),
			wantIDs: allSections,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildSettingsSchema(p, tc.snap, res, formschema.ProjectSetupState{HasOntology: true}, "")
			gotIDs := sectionIDs(got.Sections)
			assertSameSet(t, gotIDs, tc.wantIDs)
		})
	}
}

func TestBuildSettingsSchema_OwnerSeesAllSections(t *testing.T) {
	p := &domain.Project{Entity: domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}}}
	s := snap("u1", map[string]string{"project:LA": "owner"})
	got := BuildSettingsSchema(p, s, auth.Resource{
		ScopeType: "project", ID: "LA", Visibility: "private",
	}, formschema.ProjectSetupState{HasOntology: true}, "")
	gotIDs := sectionIDs(got.Sections)
	want := []string{"general", "about", "ontology", "categories", "members", "attributions", "namespace-bindings", "vocabularies", "autocomplete", "integrations"}
	assertSameSet(t, gotIDs, want)
}

func TestBuildSettingsSchema_ContributorOnlySeesCategoriesAndAttributions(t *testing.T) {
	p := &domain.Project{Entity: domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}}}
	s := snap("u1", map[string]string{"project:LA": "contributor"})
	got := BuildSettingsSchema(p, s, auth.Resource{
		ScopeType: "project", ID: "LA", Visibility: "private",
	}, formschema.ProjectSetupState{HasOntology: true}, "")
	gotIDs := sectionIDs(got.Sections)
	want := []string{"categories", "attributions"}
	if len(gotIDs) != len(want) {
		t.Errorf("contributor sections = %v, want %v", gotIDs, want)
	}
	for i, id := range want {
		if i >= len(gotIDs) || gotIDs[i] != id {
			t.Errorf("contributor sections position %d: got %v want %v", i, gotIDs, want)
		}
	}
}

func TestBuildSettingsSchema_AnonymousOnPrivateSeesNone(t *testing.T) {
	p := &domain.Project{Entity: domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}}}
	s := &auth.AuthSnapshot{IsAnonymous: true}
	got := BuildSettingsSchema(p, s, auth.Resource{
		ScopeType: "project", ID: "LA", Visibility: "private",
	}, formschema.ProjectSetupState{HasOntology: true}, "")
	if len(got.Sections) != 0 {
		t.Errorf("anon on private: got %v want no sections", sectionIDs(got.Sections))
	}
}

func TestBuildSettingsSchema_AnonymousOnPublicSeesCategories(t *testing.T) {
	p := &domain.Project{Entity: domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}}}
	s := &auth.AuthSnapshot{IsAnonymous: true}
	got := BuildSettingsSchema(p, s, auth.Resource{
		ScopeType: "project", ID: "LA", Visibility: "public",
	}, formschema.ProjectSetupState{HasOntology: true}, "")
	gotIDs := sectionIDs(got.Sections)
	want := []string{"categories", "attributions"}
	if len(gotIDs) != len(want) {
		t.Errorf("anon on public: got %v want %v", gotIDs, want)
	}
	for i, id := range want {
		if i >= len(gotIDs) || gotIDs[i] != id {
			t.Errorf("anon on public position %d: got %v want %v", i, gotIDs, want)
		}
	}
}

func TestBuildOntologySettingsSchema_OnlyParentProjectID(t *testing.T) {
	p := &domain.Project{
		Entity: domain.Entity{ID: "TEST", UIName: domain.Translations{"en": "Test"}},
	}
	schema := BuildOntologySettingsSchema(p, nil, nil, "en", nil)
	if schema == nil || len(schema.Sections) == 0 {
		t.Fatalf("expected one section, got: %+v", schema)
	}
	fields := schema.Sections[0].Fields
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Name)
	}
	if len(names) != 2 || names[0] != "parent_project_id" || names[1] != "additional_child_parents" {
		t.Errorf("expected [parent_project_id additional_child_parents], got %v", names)
	}
}

func TestBuildOntologyPaneSchema_ReferencesTwoPanels(t *testing.T) {
	pane := BuildOntologyPaneSchema("LA", "")
	if pane == nil {
		t.Fatal("nil pane")
	}
	if pane.Kind != "composite-pane" {
		t.Errorf("kind=%q want composite-pane", pane.Kind)
	}
	if len(pane.Panels) != 2 {
		t.Fatalf("panels len=%d want 2", len(pane.Panels))
	}
	if pane.Panels[0].ID != "parent-inheritance" || pane.Panels[1].ID != "linked-ontologies" {
		t.Errorf("panel IDs wrong: %v / %v", pane.Panels[0].ID, pane.Panels[1].ID)
	}
	if !strings.Contains(pane.Panels[0].SchemaURL, "/settings/list-schema/ontology-parents") {
		t.Errorf("panel[0] schema_url wrong: %s", pane.Panels[0].SchemaURL)
	}
	if pane.Panels[0].Kind != "list" {
		t.Errorf("panel[0] kind wrong: %q want list", pane.Panels[0].Kind)
	}
	if !strings.Contains(pane.Panels[1].SchemaURL, "/project-ontology-versions/pane") {
		t.Errorf("panel[1] schema_url wrong: %s", pane.Panels[1].SchemaURL)
	}
	if pane.Panels[1].Kind != "linked-ontologies" {
		t.Errorf("panel[1] kind wrong: %q want linked-ontologies", pane.Panels[1].Kind)
	}
}

func TestBuildProjectInheritanceListSchema_EditCapabilities(t *testing.T) {
	s := formschema.BuildProjectInheritanceListSchema("LA", "en", nil, "")
	if s.Caps.Create == nil || s.Caps.Reorder == nil || s.Caps.Delete == nil || s.Caps.Edit == nil {
		t.Fatalf("expected create, reorder, delete capabilities; got %#v", s.Caps)
	}
	if len(s.RowActions) != 3 {
		t.Fatalf("row actions len=%d want 3", len(s.RowActions))
	}
}

func TestBuildProjectInheritanceListSchema_ReleaseReadOnly(t *testing.T) {
	s := formschema.BuildProjectInheritanceListSchema("LA", "en", nil, "1.0.0")
	if s.Caps.Create != nil || s.Caps.Reorder != nil || s.Caps.Delete != nil {
		t.Fatalf("release schema should be read-only: %#v", s.Caps)
	}
	if len(s.RowActions) != 0 {
		t.Fatalf("release row actions len=%d want 0", len(s.RowActions))
	}
	if !strings.Contains(s.DataURL, "version=1.0.0") {
		t.Fatalf("data url %q should carry version", s.DataURL)
	}
}

func TestBuildSettingsSchema_ReleaseDraftURLGatedByEdit(t *testing.T) {
	p := &domain.Project{Entity: domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}}}
	res := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "private"}

	editor := snap("u1", map[string]string{"project:LA": "maintainer"})
	got := BuildSettingsSchema(p, editor, res, formschema.ProjectSetupState{HasOntology: true}, "1.0.0")
	if got.Release == nil {
		t.Fatalf("expected release view for a release version")
	}
	if got.Release.DraftURL == "" {
		t.Fatalf("expected editor to get a non-empty draft_url")
	}

	nonEditor := snap("u2", map[string]string{"project:LA": "contributor"})
	got = BuildSettingsSchema(p, nonEditor, res, formschema.ProjectSetupState{HasOntology: true}, "1.0.0")
	if got.Release == nil {
		t.Fatalf("expected release view for a release version")
	}
	if got.Release.DraftURL != "" {
		t.Fatalf("expected non-editor draft_url to be empty, got %q", got.Release.DraftURL)
	}
}

func TestBuildGeneralSettingsSchema_IncludesVisibilityChoices(t *testing.T) {
	p := &domain.Project{
		Entity:     domain.Entity{ID: "TPC", UIName: domain.Translations{"en": "Test Project"}},
		Visibility: "public",
	}
	schema := BuildGeneralSettingsSchema(p, false, "en", nil)
	if schema == nil {
		t.Fatal("nil schema")
	}
	var visibility *formschema.FieldDef
	for _, section := range schema.Sections {
		for i := range section.Fields {
			if section.Fields[i].Name == "visibility" {
				visibility = &section.Fields[i]
				break
			}
		}
	}
	if visibility == nil {
		t.Fatal("visibility field missing")
	}
	if visibility.Widget != formschema.WidgetRadioGroup {
		t.Fatalf("widget=%q want %q", visibility.Widget, formschema.WidgetRadioGroup)
	}
	if visibility.Value != "public" {
		t.Fatalf("value=%v want public", visibility.Value)
	}
	if len(visibility.Options) != 3 {
		t.Fatalf("options len=%d want 3", len(visibility.Options))
	}
	wantValues := []string{"private", "internal", "public"}
	for i, want := range wantValues {
		if visibility.Options[i].Value != want {
			t.Fatalf("option[%d].Value=%q want %q (full: %#v)", i, visibility.Options[i].Value, want, visibility.Options)
		}
	}
}

func TestBuildSettingsSchema_OntologyUsesPaneURL(t *testing.T) {
	p := &domain.Project{Entity: domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}}}
	s := snap("u1", map[string]string{"project:LA": "owner"})
	schema := BuildSettingsSchema(p, s, auth.Resource{
		ScopeType: "project", ID: "LA", Visibility: "private",
	}, formschema.ProjectSetupState{HasOntology: true}, "")
	var found *formschema.SettingsSection
	for i := range schema.Sections {
		if schema.Sections[i].ID == "ontology" {
			found = &schema.Sections[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected ontology section for owner")
	}
	if !strings.Contains(found.SchemaURL, "/settings/pane-schema/ontology") {
		t.Errorf("SchemaURL wrong: %s", found.SchemaURL)
	}
}

func TestBuildSettingsSchema_IncludesNamespaceBindings(t *testing.T) {
	p := &domain.Project{Entity: domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}}}
	s := snap("u1", map[string]string{"project:LA": "owner"})
	schema := BuildSettingsSchema(p, s, auth.Resource{
		ScopeType: "project", ID: "LA", Visibility: "private",
	}, formschema.ProjectSetupState{HasOntology: true}, "")
	var found *formschema.SettingsSection
	for i := range schema.Sections {
		if schema.Sections[i].ID == "namespace-bindings" {
			found = &schema.Sections[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("namespace-bindings section missing; got sections: %v", sectionIDs(schema.Sections))
	}
	if !strings.Contains(found.SchemaURL, "/namespace-bindings/list-schema") {
		t.Errorf("SchemaURL=%q want .../namespace-bindings/list-schema", found.SchemaURL)
	}
	if found.Icon != "at-symbol" {
		t.Errorf("Icon=%q want at-symbol", found.Icon)
	}
}
