package csvexport

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/internal/testsnapshot"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

func pointer[T any](v T) *T { return &v }

func exportValue(t *testing.T) projectsnapshot.CompleteProject {
	t.Helper()
	return testsnapshot.GeneratorValue(t, domain.Project{Entity: domain.Entity{ID: "G", SystemName: "captured", UIName: domain.Translations{"en": "Captured project"}}}, domain.Model{}, domain.ModelView{}, domain.Collection{Entity: domain.Entity{ID: "G.C"}}, []domain.ResolvedField{{ID: "F", SemanticID: "G.F", SystemName: "field", SetValue: "value", MaxOccurs: pointer(0)}})
}

type exportSource struct {
	v     projectsnapshot.CompleteProject
	calls int
	scope auth.ReadScope
	err   error
}

func (s *exportSource) Project(_ context.Context, scope auth.ReadScope, _ string) (projectsnapshot.CompleteProject, error) {
	s.calls++
	s.scope = scope
	return s.v, s.err
}

type projectAccess struct{}

func (projectAccess) GetByID(context.Context, string) (*domain.Project, error) {
	return &domain.Project{Entity: domain.Entity{ID: "G", SystemName: "current", UIName: domain.Translations{"en": "Current project"}}, Visibility: domain.VisibilityPublic}, nil
}

func exportRouter(t *testing.T, source *exportSource) http.Handler {
	t.Helper()
	service, err := NewService(source, projectAccess{}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Use(auth.WithProjectVersionContext)
	router.Route("/projects/{projectID}/exports", func(r chi.Router) { Mount(r, Host{Service: service}) })
	return router
}
func exportRequest(path string, member bool) *http.Request {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	return r.WithContext(auth.WithSnapshot(r.Context(), &auth.AuthSnapshot{IsSuperAdmin: member, IsAnonymous: !member}))
}
func rows(t *testing.T, data []byte) [][]string {
	t.Helper()
	r, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestZIPCapturesOnceAndMatchesSelectedFiles(t *testing.T) {
	source := &exportSource{v: exportValue(t)}
	response := httptest.NewRecorder()
	exportRouter(t, source).ServeHTTP(response, exportRequest("/projects/G/exports/all.zip", true))
	if response.Code != http.StatusOK || source.calls != 1 || source.scope.IsRelease() || !source.scope.Valid() {
		t.Fatalf("status=%d calls=%d scope=%+v body=%s", response.Code, source.calls, source.scope, response.Body)
	}
	if !strings.Contains(response.Header().Get("Content-Disposition"), "captured-export.zip") {
		t.Fatal("filename used current metadata")
	}
	archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != len(ExportTypes()) {
		t.Fatal("incomplete archive")
	}
	expected, err := Build(source.v)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range ExportTypes() {
		f, err := archive.Open("captured-" + kind + ".csv")
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		want, err := expected.CSV(kind)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, want) {
			t.Fatalf("%s differs from single export", kind)
		}
	}
	again, err := expected.ZIP()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(response.Body.Bytes(), again) {
		t.Fatal("ZIP bytes are not deterministic")
	}
}

func TestExportFailuresPublishNoDownload(t *testing.T) {
	for _, file := range []string{"all.zip", "fields.csv", ""} {
		source := &exportSource{err: &readsession.NotInReleaseError{ProjectID: "G", Version: "1.0", EntityID: "complete project"}}
		response := httptest.NewRecorder()
		exportRouter(t, source).ServeHTTP(response, exportRequest("/projects/G/exports/"+file+"?version=1.0", true))
		if response.Code != http.StatusNotFound || response.Header().Get("Content-Disposition") != "" || !strings.Contains(response.Body.String(), "not_in_release") {
			t.Fatalf("%s status=%d body=%s", file, response.Code, response.Body)
		}
		if source.calls != 1 || !source.scope.IsRelease() || source.scope.Version() != "1.0" {
			t.Fatal("selection retried or changed")
		}
	}
	for _, file := range []string{"all.zip", "fields.csv"} {
		source := &exportSource{v: exportValue(t)}
		response := httptest.NewRecorder()
		exportRouter(t, source).ServeHTTP(response, exportRequest("/projects/G/exports/"+file+"?version=1.0", true))
		if response.Code != http.StatusInternalServerError || response.Header().Get("Content-Disposition") != "" {
			t.Fatal("draft returned as release")
		}
	}
}

func TestLateProjectionFailureDoesNotPublishZIP(t *testing.T) {
	v := exportValue(t)
	root, _ := v.Root()
	c, _ := v.Content(root)
	// Structurally valid JSON, but unsuitable for the CSV translation projection.
	c.Collections[0].UIName = []byte(`["not a translation map"]`)
	source := &exportSource{v: testsnapshot.Complete(t, c)}
	response := httptest.NewRecorder()
	exportRouter(t, source).ServeHTTP(response, exportRequest("/projects/G/exports/all.zip", true))
	if response.Code != http.StatusInternalServerError || response.Header().Get("Content-Disposition") != "" || bytes.HasPrefix(response.Body.Bytes(), []byte("PK")) {
		t.Fatal("partial download published")
	}
}

func TestMembershipAndSelectedPageMetadata(t *testing.T) {
	source := &exportSource{v: exportValue(t)}
	router := exportRouter(t, source)
	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, exportRequest("/projects/G/exports/all.zip", false))
	if denied.Code != http.StatusNotFound || source.calls != 0 {
		t.Fatal("non-member reached capture")
	}
	page := httptest.NewRecorder()
	router.ServeHTTP(page, exportRequest("/projects/G/exports/", true))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Captured project") || strings.Contains(page.Body.String(), "Current project") {
		t.Fatal("page used current metadata")
	}
}

func TestProjectionPreservesRowsPastOldCategoryLimit(t *testing.T) {
	v := exportValue(t)
	root, _ := v.Root()
	c, _ := v.Content(root)
	for i := range 10001 {
		c.Categories = append(c.Categories, projectsnapshot.CategoryRecord{ID: fmt.Sprintf("G.CAT%d", i), ProjectID: "G", SemanticID: pointer(fmt.Sprintf("G.CAT%d", i))})
	}
	e, err := Build(testsnapshot.Complete(t, c))
	if err != nil {
		t.Fatal(err)
	}
	data, err := e.CSV("categories")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows(t, data)) != 10002 {
		t.Fatal("categories truncated")
	}
}

func TestOntologyColumnsUseRetainedEvidence(t *testing.T) {
	v := exportValue(t)
	root, _ := v.Root()
	in, _ := v.Input(root)
	pubs, _ := in.Evidence.Publications()
	doc, err := pubs[0].Artifact.Decode()
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(doc.BodyJSON), &body); err != nil {
		t.Fatal(err)
	}
	body["ontology"].(map[string]any)["name"] = "Retained ontology"
	version := body["version"].(map[string]any)
	version["ontology_label"] = map[string]string{"en": "Retained label"}
	version["ontology_uri"] = "https://retained.example/"
	version["version_iri"] = "https://retained.example/v1"
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	doc.BodyJSON = string(raw)
	doc.BodyDigest = fmt.Sprintf("%x", sha256.Sum256(raw))
	raw, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	pubs[0] = ontologyartifact.ProjectArtifact{Artifact: ontologyartifact.Artifact{JSON: string(raw), Digest: fmt.Sprintf("%x", sha256.Sum256(raw))}, Linked: true, Primary: true}
	refs, _ := in.Evidence.References()
	in.Evidence, err = projectsnapshot.DraftEvidence(root, in.Evidence.Descriptors(), pubs, refs)
	if err != nil {
		t.Fatal(err)
	}
	in.Content.OntologyLinks = []projectsnapshot.OntologyLinkRecord{{ProjectID: "G", OntologyVersionID: doc.VersionID, IsPrimary: pointer(true), AddedAt: "infinity", UsageNotes: pointer("Retained notes")}}
	in.Counts["weave_project_ontology_versions_archive"] = 1
	v, err = projectsnapshot.NewComplete(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := Build(v)
	if err != nil {
		t.Fatal(err)
	}
	data, err := e.CSV("ontologies")
	if err != nil {
		t.Fatal(err)
	}
	row := rows(t, data)[1]
	if row[0] != "Retained ontology" || row[1] != "Retained label" || row[3] != "https://retained.example/" || row[6] != "" || row[9] != "infinity" || row[10] != "Retained notes" {
		t.Fatalf("ontology evidence lost: %v", row)
	}
}

func TestCSVRejectsMissingScopeBeforeLoading(t *testing.T) {
	source := &exportSource{v: exportValue(t)}
	service, err := NewService(source, projectAccess{}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Route("/projects/{projectID}/exports", func(r chi.Router) { Mount(r, Host{Service: service}) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, exportRequest("/projects/G/exports/all.zip", true))
	if response.Code != http.StatusInternalServerError || source.calls != 0 || response.Header().Get("Content-Disposition") != "" {
		t.Fatal("missing scope fell through to draft")
	}
}

func TestRawOverridesAndSafeArchivePaths(t *testing.T) {
	v := exportValue(t)
	root, _ := v.Root()
	c, _ := v.Content(root)
	c.Projects[0].SystemName = pointer("../../unsafe\\path")
	c.Models = []projectsnapshot.ModelRecord{{ID: "G.M", ProjectID: "G"}}
	for i := range c.Overrides {
		c.Overrides[i].ExpectedValueType = pointer("Retained raw type")
		if c.Overrides[i].EntityType == "" {
			c.Overrides[i].SetValue = pointer("base fixed")
			c.OverrideRefs = []projectsnapshot.OverrideRefRecord{
				{ProjectID: "G", OverrideID: c.Overrides[i].ID, RefType: "resource_model", TargetID: "G.M", SemanticID: "G.M", Position: 2},
				{ProjectID: "G", OverrideID: c.Overrides[i].ID, RefType: "collection_model", TargetID: "G.C", SemanticID: "G.C", Position: 1},
			}
		}
	}
	e, err := Build(testsnapshot.Complete(t, c))
	if err != nil {
		t.Fatal(err)
	}
	fields, err := e.CSV(exportFields)
	if err != nil {
		t.Fatal(err)
	}
	if row := rows(t, fields)[1]; row[7] != "base fixed" || row[10] != "G.C,G.M" {
		t.Fatalf("base projection lost: %v", row)
	}
	base, err := e.CSV(exportBaseOverrides)
	if err != nil {
		t.Fatal(err)
	}
	if row := rows(t, base)[1]; row[5] != "Retained raw type" || row[9] != "G.C,G.M" {
		t.Fatalf("raw override changed: %v", row)
	}
	data, err := e.ZIP()
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range archive.File {
		if strings.ContainsAny(f.Name, "/\\") || strings.HasPrefix(f.Name, ".") {
			t.Fatalf("unsafe archive path: %s", f.Name)
		}
	}
}
