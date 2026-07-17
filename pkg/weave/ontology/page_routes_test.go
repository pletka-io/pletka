package ontology

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

func TestPublicOntologyPageSchemaRoutes(t *testing.T) {
	router := chi.NewRouter()
	pages := &Pages{svc: NewService(newRouteTestStore(), nil, nil)}
	pages.Mount(router)

	for _, tc := range []struct {
		name string
		path string
		kind string
	}{
		{name: "landing", path: "/ontologies/page-schema", kind: "ontology-family-landing"},
		{name: "all", path: "/ontologies/all/page-schema", kind: "ontology-all"},
		{name: "family", path: "/ontologies/families/cidoc/page-schema", kind: "ontology-family"},
		{name: "ontology", path: "/ontologies/crm/page-schema", kind: "ontology-detail"},
		{name: "version", path: "/ontologies/crm/7.1.3/page-schema", kind: "ontology-version"},
		{name: "class", path: "/ontologies/crm/7.1.3/classes/class-child/page-schema", kind: "ontology-class"},
		{name: "property", path: "/ontologies/crm/7.1.3/properties/prop-name/page-schema", kind: "ontology-property"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := requestOntologyPageSchema(t, router, tc.path)
			if schema.Kind != tc.kind {
				t.Fatalf("kind=%q want %q", schema.Kind, tc.kind)
			}
			if len(schema.Actions) != 0 && (tc.kind == "ontology-detail" || tc.kind == "ontology-version") {
				t.Fatalf("public schema has admin actions: %#v", schema.Actions)
			}
		})
	}
}

func TestPublicClassAndPropertyPageSchemaRoutesIncludeRelations(t *testing.T) {
	router := chi.NewRouter()
	pages := &Pages{svc: NewService(newRouteTestStore(), nil, nil)}
	pages.Mount(router)

	class := requestOntologyPageSchema(t, router, "/ontologies/crm/7.1.3/classes/class-child/page-schema")
	if findOntologyPageSection(class.Sections, "superclasses") == nil {
		t.Fatal("class superclasses section missing")
	}
	if findOntologyPageSection(class.Sections, "properties") == nil {
		t.Fatal("class properties section missing")
	}

	property := requestOntologyPageSchema(t, router, "/ontologies/crm/7.1.3/properties/prop-name/page-schema")
	for _, id := range []string{"domain", "range", "superproperties"} {
		if findOntologyPageSection(property.Sections, id) == nil {
			t.Fatalf("property relation section %q missing", id)
		}
	}
}

func TestPublicVersionPageSchemaRoutePreservesActiveTab(t *testing.T) {
	router := chi.NewRouter()
	pages := &Pages{svc: NewService(newRouteTestStore(), nil, nil)}
	pages.Mount(router)

	schema := requestOntologyPageSchema(t, router, "/ontologies/crm/7.1.3/page-schema?tab=properties")
	tabs := findOntologyPageSection(schema.Sections, "tabs")
	if tabs == nil {
		t.Fatal("tabs section missing")
	}
	if !hasActiveOntologyPageTab(tabs.Tabs, "properties") {
		t.Fatalf("properties tab not active: %#v", tabs.Tabs)
	}
	stats := findOntologyPageSection(schema.Sections, "stats")
	if stats == nil {
		t.Fatal("stats section missing")
	}
	if got := ontologyPageStatValue(stats.Stats, "Projects"); got != "3" {
		t.Fatalf("project usage stat=%q want 3", got)
	}
	propertiesList := findOntologyPageSection(schema.Sections, "properties-list")
	if propertiesList == nil {
		t.Fatal("properties list section missing")
	}
	if propertiesList.Widget != "entity-list" {
		t.Fatalf("properties list widget=%q want entity-list", propertiesList.Widget)
	}
	if got := propertiesList.SchemaURL; got != "/ontologies/crm/7.1.3/properties/list-schema" {
		t.Fatalf("properties list schema_url=%q", got)
	}
	if hasOntologyRouteAction(schema.Actions, "set-active") || hasOntologyRouteAction(schema.Actions, "delete") || hasOntologyRouteAction(schema.Actions, "import") {
		t.Fatalf("public version schema exposed admin actions: %#v", schema.Actions)
	}
}

func TestAdminOntologyPageSchemaRoutes(t *testing.T) {
	router := chi.NewRouter()
	handler := NewHandler(NewService(newRouteTestStore(), nil, nil), nil, nil, nil)
	router.Route("/admin/ontologies", handler.Mount)

	for _, tc := range []struct {
		name       string
		path       string
		kind       string
		actionID   string
		actionWant bool
	}{
		{name: "landing", path: "/admin/ontologies/page-schema", kind: "ontology-family-landing"},
		{name: "family", path: "/admin/ontologies/families/fam-root/page-schema", kind: "ontology-family"},
		{name: "ontology", path: "/admin/ontologies/ont-crm/page-schema", kind: "ontology-detail", actionID: "edit", actionWant: true},
		{name: "import version page", path: "/admin/ontologies/ont-crm/versions/import/page-schema", kind: "ontology-import", actionID: "back", actionWant: true},
		{name: "inactive version", path: "/admin/ontologies/versions/ver-old/page-schema", kind: "ontology-version", actionID: "set-active", actionWant: true},
		{name: "active version", path: "/admin/ontologies/versions/ver-active/page-schema", kind: "ontology-version", actionID: "set-active", actionWant: false},
		{name: "in-use version", path: "/admin/ontologies/versions/ver-old/page-schema", kind: "ontology-version", actionID: "delete", actionWant: false},
		{name: "unused version", path: "/admin/ontologies/versions/ver-unused/page-schema", kind: "ontology-version", actionID: "delete", actionWant: true},
		{name: "version import stub", path: "/admin/ontologies/versions/ver-unused/page-schema", kind: "ontology-version", actionID: "import", actionWant: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := requestOntologyPageSchema(t, router, tc.path)
			if schema.Kind != tc.kind {
				t.Fatalf("kind=%q want %q", schema.Kind, tc.kind)
			}
			if tc.actionID != "" {
				got := hasOntologyRouteAction(schema.Actions, tc.actionID)
				if got != tc.actionWant {
					t.Fatalf("action %q presence=%v want %v; actions=%#v", tc.actionID, got, tc.actionWant, schema.Actions)
				}
			}
		})
	}

	landing := requestOntologyPageSchema(t, router, "/admin/ontologies/page-schema")
	if got := firstOntologyPageCardHref(landing.Sections, "families"); got != "/admin/ontologies/families/fam-root/page" {
		t.Fatalf("admin landing first family href=%q", got)
	}
	family := requestOntologyPageSchema(t, router, "/admin/ontologies/families/fam-root/page-schema")
	if got := firstOntologyPageCardHref(family.Sections, "base-ontologies"); got != "/admin/ontologies/ont-crm/page" {
		t.Fatalf("admin family first ontology href=%q", got)
	}
	ontology := requestOntologyPageSchema(t, router, "/admin/ontologies/ont-crm/page-schema")
	if got := firstOntologyPageCardHref(ontology.Sections, "versions"); got != "/admin/ontologies/versions/ver-active/page" {
		t.Fatalf("admin ontology first version href=%q", got)
	}
	if got := findOntologyRouteAction(ontology.Actions, "edit").FormSchemaURL; got != "/admin/ontologies/form-schema?mode=edit&entity_id=ont-crm" {
		t.Fatalf("ontology edit form url=%q", got)
	}
	if got := findOntologyRouteAction(ontology.Actions, "import").Href; got != "/admin/ontologies/ont-crm/versions/import/page" {
		t.Fatalf("ontology import href=%q", got)
	}
	version := requestOntologyPageSchema(t, router, "/admin/ontologies/versions/ver-old/page-schema")
	if got := findOntologyRouteAction(version.Actions, "edit").FormSchemaURL; got != "/admin/ontologies/versions/form-schema?mode=edit&entity_id=ver-old" {
		t.Fatalf("version edit form url=%q", got)
	}
}

func TestAdminUpdateVersionParsesEditableListsAndPreservesOmittedMetadata(t *testing.T) {
	store := newRouteTestStore()
	for _, version := range store.versions {
		if version.ID == "ver-old" {
			version.CompatibleBaseVersions = []string{"7.1.3"}
			version.ImportedOntologies = []string{"crm:7.1.3"}
			version.VersionInfo = domain.Translations{"en": "Previous import"}
			version.OntologyLabel = domain.Translations{"en": "Previous label"}
			version.OntologyComment = domain.Translations{"en": "Previous comment"}
			version.OntologyMetadata = json.RawMessage(`{"source":"import"}`)
		}
	}
	router := chi.NewRouter()
	handler := NewHandler(NewService(store, nil, nil), nil, nil, nil)
	router.Route("/admin/ontologies", handler.Mount)

	body := bytes.NewBufferString(`{
		"version_string":"6.2.0",
		"compatible_base_versions_text":"7.1.4\n7.1.5",
		"imported_ontologies_text":"crm:7.1.4, crmsci:2.0",
		"ontology_uri":"http://example.org/ontology",
		"version_iri":"http://example.org/ontology/6.2.0"
	}`)
	req := httptest.NewRequest(http.MethodPut, "/admin/ontologies/versions/ver-old", body)
	req = req.WithContext(auth.WithSnapshot(req.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if store.lastVersionUpdate == nil {
		t.Fatal("update was not called")
	}
	if got := store.lastVersionUpdate.CompatibleBaseVersions; len(got) != 2 || got[0] != "7.1.4" || got[1] != "7.1.5" {
		t.Fatalf("compatible base versions=%#v, want parsed 7.1.4/7.1.5", got)
	}
	if got := store.lastVersionUpdate.ImportedOntologies; len(got) != 2 || got[0] != "crm:7.1.4" || got[1] != "crmsci:2.0" {
		t.Fatalf("imported ontologies=%#v, want parsed crm/crmsci", got)
	}
	if got := store.lastVersionUpdate.VersionInfo["en"]; got != "Previous import" {
		t.Fatalf("version info=%q, want preserved Previous import", got)
	}
	if got := store.lastVersionUpdate.OntologyLabel["en"]; got != "Previous label" {
		t.Fatalf("ontology label=%q, want preserved Previous label", got)
	}
	if got := store.lastVersionUpdate.OntologyComment["en"]; got != "Previous comment" {
		t.Fatalf("ontology comment=%q, want preserved Previous comment", got)
	}
	if string(store.lastVersionUpdate.OntologyMetadata) != `{"source":"import"}` {
		t.Fatalf("ontology metadata=%s, want preserved import metadata", string(store.lastVersionUpdate.OntologyMetadata))
	}
}

func TestAdminImportVersionUploadRoutes(t *testing.T) {
	store := newRouteTestStore()
	router := chi.NewRouter()
	handler := NewHandler(NewService(store, nil, nil), nil, nil, nil)
	router.Route("/admin/ontologies", handler.Mount)

	probe := requestMultipart(t, router, http.MethodPost, "/admin/ontologies/ont-crm/versions/import/probe", map[string]string{
		"version_string":           "9.9.9",
		"compatible_base_versions": "7.1.3",
	}, "crm_test_v9.9.9.rdf", testOntologyRDF())
	if probe.Code != http.StatusOK {
		t.Fatalf("probe status=%d body=%s", probe.Code, probe.Body.String())
	}
	var probeBody struct {
		VersionString          string   `json:"version_string"`
		ClassCount             int      `json:"class_count"`
		PropertyCount          int      `json:"property_count"`
		CompatibleBaseVersions []string `json:"compatible_base_versions"`
		CanImport              bool     `json:"can_import"`
	}
	if err := json.Unmarshal(probe.Body.Bytes(), &probeBody); err != nil {
		t.Fatalf("decode probe: %v", err)
	}
	if probeBody.VersionString != "9.9.9" || probeBody.ClassCount != 1 || probeBody.PropertyCount != 1 || !probeBody.CanImport {
		t.Fatalf("unexpected probe body: %#v", probeBody)
	}
	if len(probeBody.CompatibleBaseVersions) != 1 || probeBody.CompatibleBaseVersions[0] != "7.1.3" {
		t.Fatalf("compatible versions=%#v, want 7.1.3", probeBody.CompatibleBaseVersions)
	}

	commit := requestMultipart(t, router, http.MethodPost, "/admin/ontologies/ont-crm/versions/import", map[string]string{
		"version_string":           "9.9.9",
		"compatible_base_versions": "7.1.3",
		"set_active":               "true",
	}, "crm_test_v9.9.9.rdf", testOntologyRDF())
	if commit.Code != http.StatusCreated {
		t.Fatalf("commit status=%d body=%s", commit.Code, commit.Body.String())
	}
	if store.lastImport == nil {
		t.Fatal("import was not called")
	}
	if got := store.lastImport.Version.VersionString; got != "9.9.9" {
		t.Fatalf("import version=%q, want 9.9.9", got)
	}
	if !store.lastImport.SetActive {
		t.Fatal("import did not request set active")
	}
	if len(store.lastImport.Classes) != 1 || len(store.lastImport.Properties) != 1 {
		t.Fatalf("import counts classes=%d properties=%d", len(store.lastImport.Classes), len(store.lastImport.Properties))
	}
}

func requestMultipart(t *testing.T, handler http.Handler, method, path string, fields map[string]string, filename, content string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field: %v", err)
		}
	}
	part, err := writer.CreateFormFile("rdf_file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(auth.WithSnapshot(req.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}))
	handler.ServeHTTP(rec, req)
	return rec
}

func testOntologyRDF() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"
         xml:base="http://www.cidoc-crm.org/cidoc-crm/">
  <rdfs:Class rdf:about="http://www.cidoc-crm.org/cidoc-crm/E99_Test">
    <rdfs:label xml:lang="en">Test class</rdfs:label>
  </rdfs:Class>
  <rdf:Property rdf:about="http://www.cidoc-crm.org/cidoc-crm/P99_test">
    <rdfs:label xml:lang="en">test property</rdfs:label>
    <rdfs:domain rdf:resource="http://www.cidoc-crm.org/cidoc-crm/E99_Test"/>
  </rdf:Property>
</rdf:RDF>`
}

type ontologyRouteSchema struct {
	Kind     string                 `json:"kind"`
	Actions  []ontologyRouteAction  `json:"actions"`
	Sections []ontologyRouteSection `json:"sections"`
}

type ontologyRouteAction struct {
	ID            string `json:"id"`
	Href          string `json:"href"`
	FormSchemaURL string `json:"form_schema_url"`
}

type ontologyRouteSection struct {
	ID        string `json:"id"`
	Widget    string `json:"widget"`
	SchemaURL string `json:"schema_url"`
	Cards     []struct {
		Href string `json:"href"`
	} `json:"cards"`
	Stats []struct {
		Label map[string]string `json:"label"`
		Value string            `json:"value"`
	} `json:"stats"`
	Tabs []struct {
		ID     string `json:"id"`
		Active bool   `json:"active"`
	} `json:"tabs"`
}

func requestOntologyPageSchema(t *testing.T, handler http.Handler, path string) ontologyRouteSchema {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
	}
	var schema ontologyRouteSchema
	if err := json.Unmarshal(rec.Body.Bytes(), &schema); err != nil {
		t.Fatalf("decode schema: %v; body=%s", err, rec.Body.String())
	}
	return schema
}

func findOntologyPageSection(sections []ontologyRouteSection, id string) *ontologyRouteSection {
	for i := range sections {
		if sections[i].ID == id {
			return &sections[i]
		}
	}
	return nil
}

func firstOntologyPageCardHref(sections []ontologyRouteSection, id string) string {
	section := findOntologyPageSection(sections, id)
	if section == nil || len(section.Cards) == 0 {
		return ""
	}
	return section.Cards[0].Href
}

func hasActiveOntologyPageTab(tabs []struct {
	ID     string `json:"id"`
	Active bool   `json:"active"`
}, id string) bool {
	for _, tab := range tabs {
		if tab.ID == id && tab.Active {
			return true
		}
	}
	return false
}

func ontologyPageStatValue(stats []struct {
	Label map[string]string `json:"label"`
	Value string            `json:"value"`
}, label string) string {
	for _, stat := range stats {
		if stat.Label["en"] == label {
			return stat.Value
		}
	}
	return ""
}

func hasOntologyRouteAction(actions []ontologyRouteAction, id string) bool {
	return findOntologyRouteAction(actions, id) != nil
}

func findOntologyRouteAction(actions []ontologyRouteAction, id string) *ontologyRouteAction {
	for i := range actions {
		if actions[i].ID == id {
			return &actions[i]
		}
	}
	return nil
}

type routeTestStore struct {
	families          []*domain.OntologyFamily
	ontologies        []*domain.Ontology
	versions          []*domain.OntologyVersion
	classes           []*domain.OntologyClass
	properties        []*domain.OntologyProperty
	relations         []*domain.OntologyRelation
	usage             map[string]int64
	lastVersionUpdate *UpdateVersionMetadataInput
	lastImport        *ImportVersionInput
}

func newRouteTestStore() *routeTestStore {
	rootID := "fam-root"
	crmID := "ont-crm"
	return &routeTestStore{
		families: []*domain.OntologyFamily{
			{ID: rootID, Slug: "cidoc", Name: "CIDOC CRM", Description: domain.Translations{"en": "CIDOC family"}, DisplayOrder: 1},
			{ID: "fam-ext", Slug: "cidoc-extensions", Name: "CIDOC extensions", ParentFamilyID: &rootID, DisplayOrder: 2},
		},
		ontologies: []*domain.Ontology{
			{ID: crmID, Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/", Name: "CIDOC CRM", Description: domain.Translations{"en": "Core model"}, FamilyID: &rootID, OntologyType: domain.OntologyTypeBase},
			{ID: "ont-ext", Prefix: "crmex", Namespace: "https://example.org/crmex/", Name: "CRM Extension", FamilyID: &rootID, OntologyType: domain.OntologyTypeExtension, ExtendsOntologyID: &crmID},
		},
		versions: []*domain.OntologyVersion{
			{ID: "ver-active", OntologyID: crmID, VersionString: "7.1.3", IsActive: true, VersionInfo: domain.Translations{"en": "Current"}, ClassCount: 10, PropertyCount: 5},
			{ID: "ver-old", OntologyID: crmID, VersionString: "6.2.0", IsActive: false, VersionInfo: domain.Translations{"en": "Previous"}, ClassCount: 8, PropertyCount: 4},
			{ID: "ver-unused", OntologyID: crmID, VersionString: "5.0.0", IsActive: false, VersionInfo: domain.Translations{"en": "Unused"}, ClassCount: 6, PropertyCount: 3},
			{ID: "ver-ext", OntologyID: "ont-ext", VersionString: "1.0.0", IsActive: true, ClassCount: 2, PropertyCount: 1},
		},
		classes: []*domain.OntologyClass{
			{ID: "class-root", OntologyVersionID: "ver-active", Prefix: "crm", LocalName: "E1_CRM_Entity", URI: "http://example.org/E1", Qname: "crm:E1_CRM_Entity", Label: domain.Translations{"en": "CRM Entity"}},
			{ID: "class-child", OntologyVersionID: "ver-active", Prefix: "crm", LocalName: "E21_Person", URI: "http://example.org/E21", Qname: "crm:E21_Person", Label: domain.Translations{"en": "Person"}},
		},
		properties: []*domain.OntologyProperty{
			{ID: "prop-super", OntologyVersionID: "ver-active", Prefix: "crm", LocalName: "P1_is_identified_by", URI: "http://example.org/P1", Qname: "crm:P1_is_identified_by", Label: domain.Translations{"en": "is identified by"}},
			{ID: "prop-name", OntologyVersionID: "ver-active", Prefix: "crm", LocalName: "P131_is_identified_by", URI: "http://example.org/P131", Qname: "crm:P131_is_identified_by", Label: domain.Translations{"en": "is identified by"}},
		},
		relations: []*domain.OntologyRelation{
			{SourceID: "class-child", SourceKind: "class", RelType: "subclass_of", TargetQname: "crm:E1_CRM_Entity"},
			{SourceID: "prop-name", SourceKind: "property", RelType: "domain", TargetQname: "crm:E21_Person"},
			{SourceID: "prop-name", SourceKind: "property", RelType: "range", TargetQname: "crm:E1_CRM_Entity"},
			{SourceID: "prop-name", SourceKind: "property", RelType: "subproperty_of", TargetQname: "crm:P1_is_identified_by"},
		},
		usage: map[string]int64{
			"ver-active": 3,
			"ver-old":    1,
		},
	}
}

func (s *routeTestStore) GetFamily(_ context.Context, id string) (*domain.OntologyFamily, error) {
	for _, family := range s.families {
		if family.ID == id {
			return family, nil
		}
	}
	return nil, nil
}

func (s *routeTestStore) GetFamilyBySlug(_ context.Context, slug string) (*domain.OntologyFamily, error) {
	for _, family := range s.families {
		if family.Slug == slug {
			return family, nil
		}
	}
	return nil, nil
}

func (s *routeTestStore) ListFamilies(context.Context) ([]*domain.OntologyFamily, error) {
	return s.families, nil
}

func (s *routeTestStore) ListRootFamilies(context.Context) ([]*domain.OntologyFamily, error) {
	var out []*domain.OntologyFamily
	for _, family := range s.families {
		if family.ParentFamilyID == nil || *family.ParentFamilyID == "" {
			out = append(out, family)
		}
	}
	return out, nil
}

func (s *routeTestStore) ListChildFamilies(_ context.Context, parentID string) ([]*domain.OntologyFamily, error) {
	var out []*domain.OntologyFamily
	for _, family := range s.families {
		if family.ParentFamilyID != nil && *family.ParentFamilyID == parentID {
			out = append(out, family)
		}
	}
	return out, nil
}

func (s *routeTestStore) GetOntology(_ context.Context, id string) (*domain.Ontology, error) {
	for _, ontology := range s.ontologies {
		if ontology.ID == id {
			return ontology, nil
		}
	}
	return nil, nil
}

func (s *routeTestStore) GetOntologyByPrefix(_ context.Context, prefix string) (*domain.Ontology, error) {
	for _, ontology := range s.ontologies {
		if ontology.Prefix == prefix {
			return ontology, nil
		}
	}
	return nil, nil
}

func (s *routeTestStore) ListOntologies(context.Context) ([]*domain.Ontology, error) {
	return s.ontologies, nil
}

func (s *routeTestStore) ListOntologiesByFamily(_ context.Context, familyID string) ([]*domain.Ontology, error) {
	var out []*domain.Ontology
	for _, ontology := range s.ontologies {
		if ontology.FamilyID != nil && *ontology.FamilyID == familyID {
			out = append(out, ontology)
		}
	}
	return out, nil
}

func (s *routeTestStore) ListOntologiesByType(_ context.Context, ontologyType domain.OntologyType) ([]*domain.Ontology, error) {
	var out []*domain.Ontology
	for _, ontology := range s.ontologies {
		if ontology.OntologyType == ontologyType {
			out = append(out, ontology)
		}
	}
	return out, nil
}

func (s *routeTestStore) ListOntologyExtensions(_ context.Context, baseOntologyID string) ([]*domain.Ontology, error) {
	var out []*domain.Ontology
	for _, ontology := range s.ontologies {
		if ontology.ExtendsOntologyID != nil && *ontology.ExtendsOntologyID == baseOntologyID {
			out = append(out, ontology)
		}
	}
	return out, nil
}

func (s *routeTestStore) SearchOntologies(context.Context, string, int) ([]*domain.Ontology, error) {
	return s.ontologies, nil
}

func (s *routeTestStore) NamespaceBindings(context.Context) ([]NamespaceBinding, error) {
	return []NamespaceBinding{{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"}}, nil
}
func (s *routeTestStore) UpsertNamespaceBindings(context.Context, []NamespaceBinding) error {
	return nil
}
func (s *routeTestStore) DeleteNamespaceBindingsBySource(context.Context, string) error {
	return nil
}

func (s *routeTestStore) GetVersion(_ context.Context, id string) (*domain.OntologyVersion, error) {
	for _, version := range s.versions {
		if version.ID == id {
			return version, nil
		}
	}
	return nil, nil
}

func (s *routeTestStore) GetVersionByOntologyAndString(_ context.Context, ontologyID, versionString string) (*domain.OntologyVersion, error) {
	for _, version := range s.versions {
		if version.OntologyID == ontologyID && version.VersionString == versionString {
			return version, nil
		}
	}
	return nil, nil
}

func (s *routeTestStore) GetActiveVersion(_ context.Context, ontologyID string) (*domain.OntologyVersion, error) {
	for _, version := range s.versions {
		if version.OntologyID == ontologyID && version.IsActive {
			return version, nil
		}
	}
	return nil, nil
}

func (s *routeTestStore) ListVersionsByOntology(_ context.Context, ontologyID string) ([]*domain.OntologyVersion, error) {
	var out []*domain.OntologyVersion
	for _, version := range s.versions {
		if version.OntologyID == ontologyID {
			out = append(out, version)
		}
	}
	return out, nil
}

func (s *routeTestStore) VersionUsageCount(_ context.Context, versionID string) (int64, error) {
	return s.usage[versionID], nil
}

func (s *routeTestStore) ListProjectsUsingVersion(_ context.Context, versionID string, limit int) ([]VersionProjectUsage, error) {
	if s.usage[versionID] == 0 {
		return nil, nil
	}
	usages := []VersionProjectUsage{
		{ProjectID: "LA", ProjectName: "Linked Archive", IsPrimary: true},
		{ProjectID: "INH", ProjectName: "Inherited Heritage"},
	}
	if limit > 0 && len(usages) > limit {
		return usages[:limit], nil
	}
	return usages, nil
}

func (s *routeTestStore) VersionUsageCountsForOntology(context.Context, string) (map[string]int64, error) {
	return s.usage, nil
}

func (s *routeTestStore) GetClass(_ context.Context, id string) (*domain.OntologyClass, error) {
	for _, class := range s.classes {
		if class.ID == id {
			return class, nil
		}
	}
	return nil, nil
}
func (s *routeTestStore) GetClassByURI(_ context.Context, versionID, uri string) (*domain.OntologyClass, error) {
	for _, class := range s.classes {
		if class.OntologyVersionID == versionID && class.URI == uri {
			return class, nil
		}
	}
	return nil, nil
}
func (s *routeTestStore) GetClassByQname(_ context.Context, versionID, qname string) (*domain.OntologyClass, error) {
	for _, class := range s.classes {
		if class.OntologyVersionID == versionID && class.Qname == qname {
			return class, nil
		}
	}
	return nil, nil
}
func (s *routeTestStore) ListClassesByVersion(_ context.Context, versionID string) ([]*domain.OntologyClass, error) {
	var out []*domain.OntologyClass
	for _, class := range s.classes {
		if class.OntologyVersionID == versionID {
			out = append(out, class)
		}
	}
	return out, nil
}
func (s *routeTestStore) ListClassesByQnames(context.Context, string, []string) ([]*domain.OntologyClass, error) {
	return nil, nil
}
func (s *routeTestStore) ListSubclassesByQname(context.Context, string, string) ([]*domain.OntologyClass, error) {
	return nil, nil
}
func (s *routeTestStore) SearchClasses(context.Context, string, string, int) ([]*domain.OntologyClass, error) {
	return nil, nil
}
func (s *routeTestStore) GetProperty(_ context.Context, id string) (*domain.OntologyProperty, error) {
	for _, property := range s.properties {
		if property.ID == id {
			return property, nil
		}
	}
	return nil, nil
}
func (s *routeTestStore) GetPropertyByURI(_ context.Context, versionID, uri string) (*domain.OntologyProperty, error) {
	for _, property := range s.properties {
		if property.OntologyVersionID == versionID && property.URI == uri {
			return property, nil
		}
	}
	return nil, nil
}
func (s *routeTestStore) GetPropertyByQname(_ context.Context, versionID, qname string) (*domain.OntologyProperty, error) {
	for _, property := range s.properties {
		if property.OntologyVersionID == versionID && property.Qname == qname {
			return property, nil
		}
	}
	return nil, nil
}
func (s *routeTestStore) ListPropertiesByVersion(_ context.Context, versionID string) ([]*domain.OntologyProperty, error) {
	var out []*domain.OntologyProperty
	for _, property := range s.properties {
		if property.OntologyVersionID == versionID {
			out = append(out, property)
		}
	}
	return out, nil
}
func (s *routeTestStore) ListPropertiesByQnames(context.Context, string, []string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *routeTestStore) SearchProperties(context.Context, string, string, int) ([]*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *routeTestStore) PropertiesForDomainQname(_ context.Context, versionID, qname string) ([]*domain.OntologyProperty, error) {
	var out []*domain.OntologyProperty
	for _, rel := range s.relations {
		if rel.RelType != "domain" || rel.TargetQname != qname {
			continue
		}
		for _, property := range s.properties {
			if property.ID == rel.SourceID && property.OntologyVersionID == versionID {
				out = append(out, property)
			}
		}
	}
	return out, nil
}
func (s *routeTestStore) PropertiesForRangeQname(context.Context, string, string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *routeTestStore) GetInverseProperty(context.Context, string, string) (*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *routeTestStore) ListRelationsForSource(_ context.Context, sourceID, sourceKind string) ([]*domain.OntologyRelation, error) {
	var out []*domain.OntologyRelation
	for _, rel := range s.relations {
		if rel.SourceID == sourceID && rel.SourceKind == sourceKind {
			out = append(out, rel)
		}
	}
	return out, nil
}
func (s *routeTestStore) ListRelationsForSourceByType(_ context.Context, sourceID, sourceKind, relType string) ([]*domain.OntologyRelation, error) {
	var out []*domain.OntologyRelation
	for _, rel := range s.relations {
		if rel.SourceID == sourceID && rel.SourceKind == sourceKind && rel.RelType == relType {
			out = append(out, rel)
		}
	}
	return out, nil
}
func (s *routeTestStore) ListRelationsForTarget(_ context.Context, targetQname, relType string) ([]*domain.OntologyRelation, error) {
	var out []*domain.OntologyRelation
	for _, rel := range s.relations {
		if rel.TargetQname == targetQname && rel.RelType == relType {
			out = append(out, rel)
		}
	}
	return out, nil
}
func (s *routeTestStore) ListFieldRefs(context.Context, string) ([]*domain.FieldOntologyRef, error) {
	return nil, nil
}
func (s *routeTestStore) FindFieldsUsingQname(context.Context, string, string, int) ([]string, error) {
	return nil, nil
}
func (s *routeTestStore) CountFieldsUsingQname(context.Context, string, string) (int64, error) {
	return 0, nil
}
func (s *routeTestStore) CountFieldsUsingVersion(context.Context, string, string) (int64, error) {
	return 0, nil
}
func (s *routeTestStore) SampleFieldsUsingVersion(context.Context, string, string, int) ([]FieldVersionRef, error) {
	return nil, nil
}
func (s *routeTestStore) CreateFamily(context.Context, CreateFamilyInput) (*domain.OntologyFamily, error) {
	return nil, nil
}
func (s *routeTestStore) UpdateFamily(context.Context, string, UpdateFamilyInput) (*domain.OntologyFamily, error) {
	return nil, nil
}
func (s *routeTestStore) DeleteFamily(context.Context, string) error { return nil }
func (s *routeTestStore) CountOntologiesInFamily(context.Context, string) (int64, error) {
	return 0, nil
}
func (s *routeTestStore) CreateOntology(context.Context, CreateOntologyInput) (*domain.Ontology, error) {
	return nil, nil
}
func (s *routeTestStore) UpdateOntology(context.Context, string, UpdateOntologyInput) (*domain.Ontology, error) {
	return nil, nil
}
func (s *routeTestStore) DeleteOntology(context.Context, string) error { return nil }
func (s *routeTestStore) CountOntologyVersions(context.Context, string) (int64, error) {
	return 0, nil
}
func (s *routeTestStore) CreateVersion(context.Context, CreateVersionInput) (*domain.OntologyVersion, error) {
	return nil, nil
}
func (s *routeTestStore) UpdateVersionMetadata(_ context.Context, id string, in UpdateVersionMetadataInput) (*domain.OntologyVersion, error) {
	s.lastVersionUpdate = &in
	for _, version := range s.versions {
		if version.ID == id {
			version.VersionString = in.VersionString
			version.CompatibleBaseVersions = in.CompatibleBaseVersions
			version.OntologyURI = in.OntologyURI
			version.VersionIRI = in.VersionIRI
			version.VersionInfo = in.VersionInfo
			version.ImportedOntologies = in.ImportedOntologies
			version.OntologyLabel = in.OntologyLabel
			version.OntologyComment = in.OntologyComment
			version.OntologyMetadata = in.OntologyMetadata
			return version, nil
		}
	}
	return nil, nil
}
func (s *routeTestStore) UpdateVersionCounts(context.Context, string, int64, int64) error {
	return nil
}
func (s *routeTestStore) SetActiveVersion(context.Context, string, string) error { return nil }
func (s *routeTestStore) DeleteVersion(context.Context, string) error            { return nil }
func (s *routeTestStore) ImportVersion(_ context.Context, in ImportVersionInput) (*domain.OntologyVersion, error) {
	s.lastImport = &in
	return &domain.OntologyVersion{
		ID:                     in.Version.ID,
		OntologyID:             in.Version.OntologyID,
		VersionString:          in.Version.VersionString,
		IsActive:               in.SetActive,
		CompatibleBaseVersions: in.Version.CompatibleBaseVersions,
		ClassCount:             int64(len(in.Classes)),
		PropertyCount:          int64(len(in.Properties)),
	}, nil
}
func (s *routeTestStore) ReplaceFieldRefs(context.Context, string, string, []domain.FieldOntologyRef) error {
	return nil
}

func (s *routeTestStore) ListClassesByVersions(context.Context, []string) ([]*domain.OntologyClass, error) {
	return nil, nil
}
func (s *routeTestStore) ListPropertiesByVersions(context.Context, []string) ([]*domain.OntologyProperty, error) {
	return nil, nil
}
func (s *routeTestStore) ListRelationsByVersionsAndTypes(context.Context, []string, []string) ([]*domain.OntologyRelationWithSource, error) {
	return nil, nil
}

// TestPublicOntologyDetailShowsAdminActionsForSuperAdmin verifies the
// consolidation follow-up: the public, slug-keyed ontology
// page surfaces Edit/Import actions when the viewer is a super-admin, so
// there is no need for the separate ULID-keyed /admin/ontologies page.
func TestPublicOntologyDetailShowsAdminActionsForSuperAdmin(t *testing.T) {
	router := chi.NewRouter()
	pages := &Pages{svc: NewService(newRouteTestStore(), nil, nil)}
	pages.Mount(router)

	// Anonymous: no admin actions.
	anon := requestOntologyPageSchema(t, router, "/ontologies/crm/page-schema")
	if hasOntologyRouteAction(anon.Actions, "import") {
		t.Fatal("anonymous public page must not show the import action")
	}

	// Super-admin: import + edit appear.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ontologies/crm/page-schema", nil)
	req = req.WithContext(auth.WithSnapshot(req.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}))
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var schema ontologyRouteSchema
	if err := json.Unmarshal(rec.Body.Bytes(), &schema); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !hasOntologyRouteAction(schema.Actions, "import") {
		t.Fatalf("super-admin public page must show the import action; actions=%#v", schema.Actions)
	}
	if !hasOntologyRouteAction(schema.Actions, "edit") {
		t.Fatalf("super-admin public page must show the edit action; actions=%#v", schema.Actions)
	}
}
