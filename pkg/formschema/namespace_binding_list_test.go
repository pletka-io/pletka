package formschema_test

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// Form schema tests are colocated here to stay within the file manifest the
// plan enumerates; the two builders share a small domain and the tests are
// each only a few lines.

func TestBuildNamespaceBindingListSchema_PerRowReadonly(t *testing.T) {
	s := formschema.BuildNamespaceBindingListSchema("LA", "en", nil)
	if s == nil {
		t.Fatal("nil schema")
	}
	if s.PerRowReadonlyField != "_readonly" {
		t.Errorf("PerRowReadonlyField=%q want _readonly", s.PerRowReadonlyField)
	}
	if s.Caps.Reorder != nil {
		t.Errorf("Reorder should be nil")
	}
	if s.Caps.Create == nil || s.Caps.Edit == nil || s.Caps.Delete == nil {
		t.Errorf("expected create/edit/delete caps, got %+v", s.Caps)
	}
	if s.Caps.Stats != nil {
		t.Errorf("Stats should be nil — namespace bindings do not expose stats")
	}

	want := []string{"prefix", "namespace", "weight", "source"}
	got := make([]string, 0, len(s.Columns))
	for _, c := range s.Columns {
		got = append(got, c.Key)
	}
	if len(got) != len(want) {
		t.Fatalf("column count: got %d want %d (%v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Columns[%d].Key=%q want %q", i, got[i], w)
		}
	}
}

func TestBuildNamespaceBindingListSchema_DataAndSchemaURLs(t *testing.T) {
	s := formschema.BuildNamespaceBindingListSchema("LA", "en", nil)
	if !strings.Contains(s.DataURL, "/projects/LA/namespace-bindings") {
		t.Errorf("DataURL=%q missing expected path", s.DataURL)
	}
	if !strings.Contains(s.Caps.Create.FormSchemaURL, "mode=create") {
		t.Errorf("Create.FormSchemaURL=%q missing mode=create", s.Caps.Create.FormSchemaURL)
	}
	if !strings.Contains(s.Caps.Edit.FormSchemaURLTemplate, "mode=edit") {
		t.Errorf("Edit.FormSchemaURLTemplate=%q missing mode=edit", s.Caps.Edit.FormSchemaURLTemplate)
	}
	if !strings.Contains(s.Caps.Delete.URLTemplate, "{id}") {
		t.Errorf("Delete.URLTemplate=%q missing {id}", s.Caps.Delete.URLTemplate)
	}
}

func TestBuildNamespaceBindingListSchema_EntityType(t *testing.T) {
	s := formschema.BuildNamespaceBindingListSchema("LA", "en", nil)
	if s.EntityType != "namespace-binding" {
		t.Errorf("EntityType=%q want namespace-binding", s.EntityType)
	}
	if localizableEnglish(s.Title) == "" {
		t.Errorf("expected Title to include an English fallback")
	}
}

// TestBuildNamespaceBindingListSchema_ColumnShape makes sure column types and
// flags are what the ListManager contract requires. Prefix is primary, source
// is a badge with no numeric interpretation (tones come through at the data
// layer).
func TestBuildNamespaceBindingListSchema_ColumnShape(t *testing.T) {
	s := formschema.BuildNamespaceBindingListSchema("LA", "en", nil)
	byKey := make(map[string]formschema.Column, len(s.Columns))
	for _, c := range s.Columns {
		byKey[c.Key] = c
	}
	if !byKey["prefix"].Primary {
		t.Errorf("prefix column should be Primary")
	}
	if !byKey["namespace"].Secondary {
		t.Errorf("namespace column should be Secondary")
	}
	if byKey["weight"].Type != "badge" {
		t.Errorf("weight column type=%q want badge", byKey["weight"].Type)
	}
	if byKey["source"].Type != "text_badge" {
		t.Errorf("source column type=%q want text_badge", byKey["source"].Type)
	}
}

// Sanity check: ensure the translated labels satisfy the Localizable contract
// and expose an English fallback regardless of whether the concrete type is
// raw Translations or i18n.LocalizedText.
func TestBuildNamespaceBindingListSchema_Translations(t *testing.T) {
	s := formschema.BuildNamespaceBindingListSchema("LA", "en", nil)
	var _ domain.Localizable = s.Title
	if localizableEnglish(s.Title) == "" {
		t.Errorf("Title English fallback should not be empty")
	}
}

func localizableEnglish(v domain.Localizable) string {
	switch t := any(v).(type) {
	case domain.Translations:
		return t.Get("en")
	case i18n.LocalizedText:
		return t.Translations.Get("en")
	default:
		return ""
	}
}

func TestBuildNamespaceBindingCreateForm(t *testing.T) {
	f := formschema.BuildNamespaceBindingCreateForm("LA", "en", nil)
	if f == nil {
		t.Fatal("nil form")
	}
	if f.Mode != formschema.ModeCreate {
		t.Errorf("Mode=%q want create", f.Mode)
	}
	if f.Endpoint == nil || f.Endpoint.Method != "POST" {
		t.Errorf("Endpoint=%+v want POST", f.Endpoint)
	}
	if !strings.Contains(f.Endpoint.URL, "/projects/LA/namespace-bindings") {
		t.Errorf("Endpoint.URL=%q missing expected path", f.Endpoint.URL)
	}

	wantFields := []string{"prefix", "namespace", "weight"}
	got := collectNamespaceBindingFieldNames(f)
	if len(got) != len(wantFields) {
		t.Fatalf("field count: got %d want %d (%v)", len(got), len(wantFields), got)
	}
	for i, w := range wantFields {
		if got[i] != w {
			t.Errorf("fields[%d]=%q want %q", i, got[i], w)
		}
	}
}

func TestBuildNamespaceBindingEditForm(t *testing.T) {
	b := &domain.NamespaceBinding{
		ID:        "01J000000000000000000000BN",
		Prefix:    "crm",
		Namespace: "http://www.cidoc-crm.org/cidoc-crm/",
		Weight:    5,
		Source:    "user",
	}
	f := formschema.BuildNamespaceBindingEditForm("LA", b, "en", nil)
	if f == nil {
		t.Fatal("nil form")
	}
	if f.Mode != formschema.ModeEdit {
		t.Errorf("Mode=%q want edit", f.Mode)
	}
	if f.Endpoint == nil || f.Endpoint.Method != "PATCH" {
		t.Errorf("Endpoint=%+v want PATCH", f.Endpoint)
	}
	if !strings.Contains(f.Endpoint.URL, "/projects/LA/namespace-bindings/"+b.ID) {
		t.Errorf("Endpoint.URL=%q missing expected path", f.Endpoint.URL)
	}

	wantFields := []string{"prefix", "namespace", "weight"}
	got := collectNamespaceBindingFieldNames(f)
	if len(got) != len(wantFields) {
		t.Fatalf("field count: got %d want %d (%v)", len(got), len(wantFields), got)
	}
	for i, w := range wantFields {
		if got[i] != w {
			t.Errorf("fields[%d]=%q want %q", i, got[i], w)
		}
	}

	// Values pre-populated from the binding.
	byName := indexNamespaceBindingFields(f)
	if byName["prefix"].Value != b.Prefix {
		t.Errorf("prefix.Value=%v want %q", byName["prefix"].Value, b.Prefix)
	}
	if byName["namespace"].Value != b.Namespace {
		t.Errorf("namespace.Value=%v want %q", byName["namespace"].Value, b.Namespace)
	}
	if byName["weight"].Value != b.Weight {
		t.Errorf("weight.Value=%v want %d", byName["weight"].Value, b.Weight)
	}
}

func collectNamespaceBindingFieldNames(f *formschema.FormSchema) []string {
	var names []string
	for _, s := range f.Sections {
		for _, fd := range s.Fields {
			names = append(names, fd.Name)
		}
	}
	return names
}

func indexNamespaceBindingFields(f *formschema.FormSchema) map[string]formschema.FieldDef {
	out := make(map[string]formschema.FieldDef)
	for _, s := range f.Sections {
		for _, fd := range s.Fields {
			out[fd.Name] = fd
		}
	}
	return out
}
