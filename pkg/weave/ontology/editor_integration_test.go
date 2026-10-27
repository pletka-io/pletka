//go:build integration

package ontology_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

const editorURL = "/api/v2/ontologies/EDITOR_ONTOLOGY/draft"

func editorCall(t *testing.T, router http.Handler, method, path string, body any, admin bool) *httptest.ResponseRecorder {
	t.Helper()
	var input bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&input).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, &input)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req = req.WithContext(auth.WithSnapshot(req.Context(), &auth.AuthSnapshot{IsSuperAdmin: admin, IsAnonymous: !admin}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}
func expectEditorStatus(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("HTTP %d want %d: %s", response.Code, status, response.Body.String())
	}
}
func editorSnapshot(t *testing.T, response *httptest.ResponseRecorder) ontology.DraftSnapshot {
	t.Helper()
	var snapshot ontology.DraftSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}
func editorFormValues(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	expectEditorStatus(t, response, http.StatusOK)
	var schema struct {
		Sections []struct {
			Fields []struct {
				Name  string
				Value any
			}
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &schema); err != nil {
		t.Fatal(err)
	}
	values := map[string]any{}
	for _, section := range schema.Sections {
		for _, field := range section.Fields {
			values[field.Name] = field.Value
		}
	}
	return values
}
func TestOntologyEditorHTTPDraftLifecycle(t *testing.T) {
	pool, store, drafts, document, bus := draftFixture(t)
	document.Bindings = []ontology.DraftNamespace{{Prefix: "retained", Namespace: "https://retained/", Weight: 80, Source: "manifest"}}
	service := ontology.NewService(store, nil, nil)
	host := &ontology.EditorHost{Drafts: drafts, Catalog: service, Releases: ontologyrelease.NewFactory(pool)}
	router := chi.NewRouter()
	ontology.MountEditorAPI(router, host)
	for _, request := range [][2]string{{"GET", editorURL}, {"POST", editorURL}, {"GET", editorURL + "/page-schema"}, {"GET", editorURL + "/start-form-schema"}, {"POST", editorURL + "/form"}, {"GET", editorURL + "/missing"}, {"GET", editorURL + "/missing/form-schema"}, {"PUT", editorURL + "/missing/form"}, {"PUT", editorURL + "/missing"}, {"GET", editorURL + "/missing/publish-form-schema"}, {"POST", editorURL + "/missing/publish"}} {
		response := editorCall(t, router, request[0], request[1], map[string]any{}, false)
		expectEditorStatus(t, response, http.StatusNotFound)
		if !strings.Contains(response.Body.String(), `"code":"not_found"`) {
			t.Fatal("auth response lost canonical shape")
		}
	}
	nonAdmin := httptest.NewRequestWithContext(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{}), http.MethodGet, editorURL, nil)
	nonAdmin.Header.Set("Accept", "application/json")
	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, nonAdmin)
	expectEditorStatus(t, denied, http.StatusNotFound)
	expectEditorStatus(t, editorCall(t, router, "GET", editorURL, nil, true), http.StatusNotFound)
	page := editorCall(t, router, "GET", editorURL+"/page-schema", nil, true)
	expectEditorStatus(t, page, http.StatusOK)
	if !strings.Contains(page.Body.String(), "create-draft") {
		t.Fatal("empty draft has no create action")
	}
	// Starting from an explicit document does not choose active history.
	response := editorCall(t, router, "POST", editorURL, map[string]any{"document": document}, true)
	expectEditorStatus(t, response, http.StatusCreated)
	snapshot := editorSnapshot(t, response)
	bound := editorURL + "/" + string(snapshot.ID)
	expectEditorStatus(t, editorCall(t, router, "POST", editorURL, map[string]any{"document": document}, true), http.StatusConflict)
	form := editorCall(t, router, "GET", bound+"/form-schema", nil, true)
	values := editorFormValues(t, form)
	if !strings.Contains(form.Body.String(), `"revision_field":"revision"`) {
		t.Fatal("editor lacks concurrency contract")
	}
	values["name"] = "Editor name"
	values["source_0_content"] = "unfinished RDF"
	response = editorCall(t, router, "PUT", bound+"/form", values, true)
	expectEditorStatus(t, response, http.StatusOK)
	snapshot = editorSnapshot(t, response)
	if snapshot.Revision != 2 || snapshot.Document.Ontology.Name != "Editor name" || snapshot.Document.Primary.Content != "unfinished RDF" {
		t.Fatal("form did not save draft values")
	}
	live, err := store.GetOntology(t.Context(), string(draftOwner))
	if err != nil || live.Name != "Live catalog" {
		t.Fatalf("draft changed live catalog: %v", err)
	}
	expectEditorStatus(t, editorCall(t, router, "PUT", bound+"/form", values, true), http.StatusConflict)
	expectEditorStatus(t, editorCall(t, router, "POST", bound+"/publish", map[string]any{"revision": 2, "version": "1"}, true), http.StatusUnprocessableEntity)
	if len(bus.events) != 0 {
		t.Fatal("failed publication emitted an event")
	}
	// All source operations use the same expected revision. Unexposed settings
	// survive this form adapter; fields never send replacement version rows.
	values = editorFormValues(t, editorCall(t, router, "GET", bound+"/form-schema", nil, true))
	values["source_0_content"] = draftPrimary
	values["source_1_remove"] = true
	values["new_filename"] = "extra.rdf"
	values["new_content"] = `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"/>`
	values["new_description"] = "New companion"
	values["is_active"] = true
	expectEditorStatus(t, editorCall(t, router, "PUT", bound+"/form", values, true), http.StatusUnprocessableEntity)
	delete(values, "is_active")
	response = editorCall(t, router, "PUT", bound+"/form", values, true)
	expectEditorStatus(t, response, http.StatusOK)
	snapshot = editorSnapshot(t, response)
	if snapshot.Revision != 3 || len(snapshot.Document.Companions) != 1 || snapshot.Document.Companions[0].Filename != "extra.rdf" {
		t.Fatal("companion editing did not stay in the draft")
	}
	if len(snapshot.Document.Bindings) != 1 || snapshot.Document.Bindings[0].Source != "manifest" {
		t.Fatal("form erased unexposed namespace provenance")
	}
	if snapshot.Publication != nil {
		t.Fatal("unsaved publication evidence")
	}
	// Canonical JSON update also requires a bound owner and revision.
	expectEditorStatus(t, editorCall(t, router, "PUT", strings.Replace(bound, "EDITOR_ONTOLOGY", "wrong-owner", 1), map[string]any{"revision": 3, "document": document}, true), http.StatusNotFound)
	expectEditorStatus(t, editorCall(t, router, "GET", editorURL+"/not-a-draft", nil, true), http.StatusNotFound)
	response = editorCall(t, router, "POST", bound+"/publish", map[string]any{"revision": 3, "version": "1", "set_active": true}, true)
	expectEditorStatus(t, response, http.StatusCreated)
	var publication ontology.DraftPublication
	if err := json.Unmarshal(response.Body.Bytes(), &publication); err != nil {
		t.Fatal(err)
	}
	response = editorCall(t, router, "GET", bound, nil, true)
	expectEditorStatus(t, response, http.StatusOK)
	snapshot = editorSnapshot(t, response)
	if snapshot.Publication == nil || snapshot.Publication.Version != "1" || snapshot.Publication.Digest != publication.Digest {
		t.Fatal("read did not expose this revision's receipt")
	}
	response = editorCall(t, router, "GET", editorURL+"/page-schema", nil, true)
	if strings.Contains(response.Body.String(), "publish-draft") || !strings.Contains(response.Body.String(), "Published as 1") {
		t.Fatal("published revision offers publish again")
	}
	response = editorCall(t, router, "GET", bound+"/publish-form-schema", nil, true)
	if strings.Contains(response.Body.String(), `"endpoint"`) {
		t.Fatal("published revision form remains writable")
	}
	expectEditorStatus(t, editorCall(t, router, "POST", bound+"/publish", map[string]any{"revision": 3, "version": "1", "set_active": true}, true), http.StatusOK)
	if len(bus.events) != 1 {
		t.Fatal("retry emitted a second event")
	}
	// A normal JSON save starts a new unpublished revision; typed input rejects
	// unknown fields and cannot rewrite the immutable receipt/version identity.
	document.Ontology.Name = "Next draft"
	response = editorCall(t, router, "PUT", bound, map[string]any{"revision": 3, "document": document}, true)
	expectEditorStatus(t, response, http.StatusOK)
	if editorSnapshot(t, response).Publication != nil {
		t.Fatal("new edit inherited the old receipt")
	}
	response = editorCall(t, router, "PUT", bound, map[string]any{"revision": 4, "document": document, "version_id": publication.VersionID}, true)
	expectEditorStatus(t, response, http.StatusBadRequest)
	// /api/ bypasses the generic CSRF token middleware, so require JSON and
	// reject browser-simple text/plain requests before any mutation.
	req := httptest.NewRequestWithContext(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), "POST", bound+"/publish", strings.NewReader(`{"revision":4,"version":"csrf"}`))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Accept", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, req)
	expectEditorStatus(t, response, http.StatusBadRequest)
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("editor response may be cached")
	}
}

func TestOntologyEditorFormsStartExplicitlyAndKeepReleasesReadOnly(t *testing.T) {
	pool, store, drafts, document, _ := draftFixture(t)
	service := ontology.NewService(store, nil, nil)
	host := &ontology.EditorHost{Drafts: drafts, Catalog: service, Releases: ontologyrelease.NewFactory(pool)}
	router := chi.NewRouter()
	ontology.MountEditorAPI(router, host)
	values := editorFormValues(t, editorCall(t, router, "GET", editorURL+"/start-form-schema", nil, true))
	values["source_0_content"] = draftPrimary
	response := editorCall(t, router, "POST", editorURL+"/form", values, true)
	expectEditorStatus(t, response, http.StatusCreated)
	snapshot := editorSnapshot(t, response)
	publication, err := drafts.Publish(t.Context(), draftRef(t, snapshot), snapshot.Revision, ontology.DraftPublishRequest{Version: "source"})
	if err != nil {
		t.Fatal(err)
	}
	admin := chi.NewRouter()
	ontology.MountAdmin(admin, ontology.AdminHost{Service: service, Editor: host})
	response = editorCall(t, admin, "GET", "/versions/"+publication.VersionID+"/page-schema", nil, true)
	expectEditorStatus(t, response, http.StatusOK)
	var page struct{ Actions []struct{ ID string } }
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	for _, action := range page.Actions {
		if action.ID == "edit" || action.ID == "delete" {
			t.Fatalf("immutable version offers %s", action.ID)
		}
	}
	response = editorCall(t, admin, "GET", "/versions/form-schema?entity_id="+publication.VersionID, nil, true)
	expectEditorStatus(t, response, http.StatusOK)
	if strings.Contains(response.Body.String(), `"endpoint"`) {
		t.Fatal("version metadata form can submit")
	}
	// An owner mismatch is rejected before a start-from-release form can mint a capability.
	if _, err := store.CreateOntology(t.Context(), ontology.CreateOntologyInput{ID: "FOREIGN_EDITOR", Prefix: "foreign", Name: "Foreign", Namespace: "https://foreign/", OntologyType: domain.OntologyTypeBase}); err != nil {
		t.Fatal(err)
	}
	foreign := strings.Replace(editorURL, "EDITOR_ONTOLOGY", "FOREIGN_EDITOR", 1)
	expectEditorStatus(t, editorCall(t, router, "GET", foreign+"/start-form-schema?from_release="+publication.VersionID, nil, true), http.StatusNotFound)
	expectEditorStatus(t, editorCall(t, router, "POST", foreign, map[string]any{"from_release": publication.VersionID}, true), http.StatusNotFound)
	// The existing draft is never replaced by a new source request.
	expectEditorStatus(t, editorCall(t, router, "POST", editorURL, map[string]any{"from_release": publication.VersionID}, true), http.StatusConflict)
	// Isolated destination without a draft exercises the real source-release form.
	otherPool, otherStore, otherDrafts, _, _ := draftFixture(t)
	parsed, err := ontology.NewImportRDFParser(nil).Parse(strings.NewReader(document.Primary.Content), document.Primary.Filename, int64(len(document.Primary.Content)))
	if err != nil {
		t.Fatal(err)
	}
	input := ontology.BuildImportVersionInput(&domain.Ontology{ID: string(draftOwner), Prefix: "ed", Namespace: "https://editor.example/"}, ontology.ImportVersionBuildOptions{VersionID: ontology.GenerateVersionID("ed", "base"), VersionString: "base"}, parsed, ontology.BuildImportNamespaceResolver(nil))
	version, err := otherStore.ImportVersion(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	other := chi.NewRouter()
	ontology.MountEditorAPI(other, &ontology.EditorHost{Drafts: otherDrafts, Catalog: ontology.NewService(otherStore, nil, nil), Releases: ontologyrelease.NewFactory(otherPool)})
	startValues := editorFormValues(t, editorCall(t, other, "GET", editorURL+"/start-form-schema?from_release="+version.ID, nil, true))
	response = editorCall(t, other, "POST", editorURL, startValues, true)
	expectEditorStatus(t, response, http.StatusCreated)
	copied := editorSnapshot(t, response)
	if copied.BaseVersionID == nil || *copied.BaseVersionID != version.ID || copied.Document.Primary.Content != draftPrimary {
		t.Fatal("source form did not copy frozen inputs")
	}
}
