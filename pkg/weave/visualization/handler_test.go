package visualization

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/exportgraph"
)

// superAdminRequest builds a GET to url with a superadmin snapshot in
// the request context. The export-graph routes gate on superadmin
// (defense-in-depth alongside the schema URL hiding); tests need to
// pass that gate before they exercise the actual handler logic.
func superAdminRequest(method, url string) *http.Request {
	req := httptest.NewRequest(method, url, nil)
	snap := &auth.AuthSnapshot{IsSuperAdmin: true}
	return req.WithContext(auth.WithSnapshot(req.Context(), snap))
}

func TestExportGraphRoutesRenderModelGraph(t *testing.T) {
	project := domain.Project{Entity: domain.Entity{ID: "LA"}, Visibility: "public"}
	model := domain.Model{
		Entity:        domain.Entity{ID: "model-1", SemanticID: "LAM.1", SystemName: "person", ProjectID: "LA"},
		OntologyScope: classElement("E21_Person"),
	}
	collection := domain.Collection{
		Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "name_collection", ProjectID: "LA"},
		OntologyScope: classElement("E41_Appellation"),
	}
	sharedPrefix := []domain.PathElement{
		classElement("E21_Person"),
		propertyElement("P1_is_identified_by"),
		classElement("E41_Appellation"),
	}
	field := resolvedGraphField("field-1", "LAF.1", "name", sharedPrefix)
	modelView := domain.ModelView{
		ModelID:   model.ID,
		ProjectID: project.ID,
		Categories: []domain.CategoryGroup{{
			ID:       "identity",
			Name:     domain.Translations{"en": "Identity"},
			Position: 1,
			Collections: []domain.CollectionGroup{{
				ID:               collection.ID,
				Name:             domain.Translations{"en": "Names"},
				Position:         1,
				SharedPathPrefix: sharedPrefix,
				Fields:           []domain.ResolvedField{field},
			}},
		}},
	}

	router := testVisualizationRouter(t, project, model, modelView, collection, nil)
	rec := httptest.NewRecorder()
	req := superAdminRequest(http.MethodGet, "/gen/models/model-1/exportgraph")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	var body struct {
		RootKey string `json:"root_key"`
		Nodes   []struct {
			Key   string             `json:"key"`
			Class domain.PathElement `json:"class"`
		} `json:"nodes"`
		Groups []struct {
			Kind      string `json:"kind"`
			BranchKey string `json:"branch_key"`
		} `json:"groups"`
		Fields []struct {
			SemanticID string `json:"semantic_id"`
			NodeKey    string `json:"node_key"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal graph: %v\n%s", err, rec.Body.String())
	}
	if body.RootKey == "" || len(body.Nodes) < 2 {
		t.Fatalf("unexpected graph shape: %+v", body)
	}
	if body.Nodes[0].Class.URI != "crm:E21_Person" {
		t.Fatalf("root class = %q, want crm:E21_Person", body.Nodes[0].Class.URI)
	}
	if len(body.Groups) == 0 {
		t.Fatalf("expected visual groups in model export graph")
	}
	if len(body.Fields) != 1 || body.Fields[0].SemanticID != "LAF.1" || body.Fields[0].NodeKey == "" {
		t.Fatalf("unexpected field binding: %+v", body.Fields)
	}
}

func TestExportGraphRoutesRenderCollectionGraph(t *testing.T) {
	project := domain.Project{Entity: domain.Entity{ID: "LA"}, Visibility: "public"}
	collection := domain.Collection{
		Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "birth", ProjectID: "LA"},
		OntologyScope: classElement("E67_Birth"),
	}
	fields := []domain.ResolvedField{
		resolvedGraphField("field-1", "LAF.2", "birth_place", []domain.PathElement{
			classElement("E67_Birth"),
			propertyElement("P7_took_place_at"),
			classElement("E53_Place"),
		}),
	}

	router := testVisualizationRouter(t, project, domain.Model{}, domain.ModelView{}, collection, fields)
	rec := httptest.NewRecorder()
	req := superAdminRequest(http.MethodGet, "/gen/collections/collection-1/exportgraph")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	var body struct {
		RootKey string `json:"root_key"`
		Nodes   []struct {
			Class domain.PathElement `json:"class"`
		} `json:"nodes"`
		Fields []struct {
			SemanticID string `json:"semantic_id"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal graph: %v\n%s", err, rec.Body.String())
	}
	if body.RootKey == "" || len(body.Nodes) < 2 {
		t.Fatalf("unexpected graph shape: %+v", body)
	}
	if body.Nodes[0].Class.URI != "crm:E67_Birth" {
		t.Fatalf("root class = %q, want crm:E67_Birth", body.Nodes[0].Class.URI)
	}
	if len(body.Fields) != 1 || body.Fields[0].SemanticID != "LAF.2" {
		t.Fatalf("unexpected field binding: %+v", body.Fields)
	}
}

func testVisualizationRouter(t *testing.T, project domain.Project, model domain.Model, modelView domain.ModelView, collection domain.Collection, collectionFields []domain.ResolvedField) chi.Router {
	t.Helper()
	registry, err := generators.NewRegistry(exportgraph.NewRenderer())
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	projectReader := &testProjectReader{project: project}
	modelReader := &testModelReader{model: model, view: modelView}
	collectionReader := &testCollectionReader{collection: collection, fields: collectionFields}
	namespaceReader := &testNamespaceReader{}
	gens := generators.NewService(projectReader, modelReader, collectionReader, nil, namespaceReader, registry)
	handler := NewHandler(
		testWeaveStore{
			project:    project,
			model:      model,
			collection: collection,
		},
		gens,
		nil, // ontologyBundleReader — x3ml routes not exercised here
		nil,
		nil, // latestRelease — release default not exercised here
	)
	router := chi.NewMux()
	router.Get("/gen/models/{modelID}/exportgraph", handler.GetModelExportGraph)
	router.Get("/gen/collections/{collectionID}/exportgraph", handler.GetCollectionExportGraph)
	return router
}

type testWeaveStore struct {
	domain.WeaveStore
	project    domain.Project
	model      domain.Model
	collection domain.Collection
}

func (s testWeaveStore) Projects() domain.WeaveProjectStore {
	return testProjectStore{project: s.project}
}

func (s testWeaveStore) Models() domain.WeaveModelStore {
	return testWeaveModelStore{model: s.model}
}

func (s testWeaveStore) Collections() domain.WeaveCollectionStore {
	return testWeaveCollectionStore{collection: s.collection}
}

type testProjectStore struct {
	domain.WeaveProjectStore
	project domain.Project
}

func (s testProjectStore) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	if s.project.ID != id {
		return nil, nil
	}
	return &s.project, ctx.Err()
}

type testWeaveModelStore struct {
	domain.WeaveModelStore
	model domain.Model
}

func (s testWeaveModelStore) GetByID(ctx context.Context, id string) (*domain.Model, error) {
	if s.model.ID != id {
		return nil, nil
	}
	return &s.model, ctx.Err()
}

type testWeaveCollectionStore struct {
	domain.WeaveCollectionStore
	collection domain.Collection
}

func (s testWeaveCollectionStore) GetByID(ctx context.Context, id string) (*domain.Collection, error) {
	if s.collection.ID != id {
		return nil, nil
	}
	return &s.collection, ctx.Err()
}

type testProjectReader struct {
	project domain.Project
}

func (r *testProjectReader) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	if r.project.ID != id {
		return nil, nil
	}
	return &r.project, ctx.Err()
}

func (r *testProjectReader) ResolvedOntologyVersions(ctx context.Context, projectID string, opts domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error) {
	return nil, ctx.Err()
}

type testModelReader struct {
	model domain.Model
	view  domain.ModelView
}

func (r *testModelReader) Get(ctx context.Context, projectID, id string) (*domain.Model, error) {
	if r.model.ID != id {
		return nil, nil
	}
	return &r.model, ctx.Err()
}

func (r *testModelReader) View(ctx context.Context, projectID, id string) (*domain.ModelView, error) {
	return &r.view, ctx.Err()
}

type testCollectionReader struct {
	collection domain.Collection
	fields     []domain.ResolvedField
}

func (r *testCollectionReader) Get(ctx context.Context, projectID, id string) (*domain.Collection, error) {
	if r.collection.ID != id {
		return nil, nil
	}
	return &r.collection, ctx.Err()
}

func (r *testCollectionReader) View(ctx context.Context, projectID, id string) ([]domain.ResolvedField, error) {
	return r.fields, ctx.Err()
}

type testNamespaceReader struct{}

func (r *testNamespaceReader) ListForProject(ctx context.Context, projectID string) ([]*domain.NamespaceBinding, error) {
	return nil, ctx.Err()
}

func resolvedGraphField(id, semanticID, name string, path []domain.PathElement) domain.ResolvedField {
	return domain.ResolvedField{
		ID:           id,
		SemanticID:   semanticID,
		SystemName:   name,
		DisplayName:  domain.Translations{"en": name},
		PathElements: path,
	}
}

func classElement(localName string) domain.PathElement {
	return domain.PathElement{
		Type:      "class",
		URI:       "crm:" + localName,
		Prefix:    "crm",
		LocalName: localName,
		ClassCode: localName[:3],
	}
}

func propertyElement(localName string) domain.PathElement {
	return domain.PathElement{
		Type:      "property",
		URI:       "crm:" + localName,
		Prefix:    "crm",
		LocalName: localName,
	}
}
