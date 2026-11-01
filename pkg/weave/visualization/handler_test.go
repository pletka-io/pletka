package visualization

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/internal/testsnapshot"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/exportgraph"
	"github.com/pletka-io/pletka/pkg/weave/generators/mermaid"
)

// superAdminRequest builds a GET to url with a superadmin snapshot in
// the request context. The export-graph routes gate on superadmin
// (defense-in-depth alongside the schema URL hiding); tests need to
// pass that gate before they exercise the actual handler logic.
func superAdminRequest(method, url string) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), method, url, nil)
	snap := &auth.AuthSnapshot{IsSuperAdmin: true}
	return req.WithContext(auth.WithSnapshot(req.Context(), snap))
}

type failedReleaseReader struct{ err error }

func (f failedReleaseReader) LatestReleaseVersion(context.Context, string) (string, error) {
	return "", f.err
}

func TestReleaseLookupFailureStopsEntityReads(t *testing.T) {
	cause := errors.New("release lookup failed")
	project := domain.Project{Entity: domain.Entity{ID: "P"}, Visibility: domain.VisibilityPublic}
	entity := domain.Entity{ID: "E", ProjectID: project.ID}
	h := NewHandler(testWeaveStore{
		project: project, model: domain.Model{Entity: entity},
		collection: domain.Collection{Entity: entity}, field: domain.Field{Entity: entity},
	}, nil, nil, failedReleaseReader{err: cause})
	for _, kind := range []string{"model", "collection", "field"} {
		t.Run(kind, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/gen/"+kind+"/E", nil)
			ctx := auth.WithSnapshot(req.Context(), &auth.AuthSnapshot{IsAnonymous: true})
			route := chi.NewRouteContext()
			route.URLParams.Add(kind+"ID", "E")
			ctx = context.WithValue(ctx, chi.RouteCtxKey, route)
			ctx, slot := errresp.WithCauseSlot(ctx)
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()
			var ok bool
			switch kind {
			case "model":
				_, _, ok = h.loadModelAndGate(rr, req)
			case "collection":
				_, _, ok = h.loadCollectionAndGate(rr, req)
			case "field":
				_, _, ok = h.loadFieldAndGate(rr, req)
			}
			if ok || rr.Code != http.StatusInternalServerError || !errors.Is(slot.Err(), cause) {
				t.Fatalf("ok=%v, status=%d, cause=%v; want failure with lookup cause", ok, rr.Code, slot.Err())
			}
		})
	}
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
	if body.Nodes[0].Class.URI != "http://www.cidoc-crm.org/cidoc-crm/E21_Person" {
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
	if body.Nodes[0].Class.URI != "http://www.cidoc-crm.org/cidoc-crm/E67_Birth" {
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
	source := testsnapshot.Source{Value: testsnapshot.GeneratorValue(t, project, model, modelView, collection, collectionFields)}
	gens := generators.NewService(source, registry)

	handler := NewHandler(
		testWeaveStore{
			project:    project,
			model:      model,
			collection: collection,
		},
		gens,
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
	field      domain.Field
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

func (s testWeaveStore) WeaveFields() domain.WeaveFieldStore {
	return testWeaveFieldStore{field: s.field}
}

type testWeaveFieldStore struct {
	domain.WeaveFieldStore
	field domain.Field
}

func (s testWeaveFieldStore) GetByID(ctx context.Context, id string) (*domain.Field, error) {
	if s.field.ID != id {
		return nil, nil
	}
	return &s.field, ctx.Err()
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

func TestDiagramNameComesFromSelectedInput(t *testing.T) {
	project := domain.Project{Entity: domain.Entity{ID: "G"}, Visibility: domain.VisibilityPublic}
	model := domain.Model{Entity: domain.Entity{ID: "M", ProjectID: "G", UIName: domain.Translations{"en": "Captured name"}}}
	value := testsnapshot.GeneratorValue(t, project, model, domain.ModelView{}, domain.Collection{}, nil)
	model.UIName = domain.Translations{"en": "Current name"}
	registry, err := generators.NewRegistry(mermaid.NewRenderer())
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(testWeaveStore{project: project, model: model}, generators.NewService(testsnapshot.Source{Value: value}, registry), nil, nil)
	router := chi.NewRouter()
	router.Get("/gen/models/{modelID}/diagram", handler.GetModelDiagram)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, superAdminRequest(http.MethodGet, "/gen/models/M/diagram"))
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var body DiagramResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Name != "Captured name" {
		t.Fatalf("diagram name from current data: %q", body.Name)
	}
}

type unavailableSelection struct {
	testsnapshot.Source
	scope auth.ReadScope
	calls int
}

func (s *unavailableSelection) Entity(_ context.Context, scope auth.ReadScope, project, _ string, id string) (projectsnapshot.EntityFragment, error) {
	s.scope = scope
	s.calls++
	return projectsnapshot.EntityFragment{}, &readsession.NotInReleaseError{ProjectID: project, Version: scope.Version(), EntityID: id}
}

func TestDiagramForwardsReleaseAndPreservesUnavailableError(t *testing.T) {
	project := domain.Project{Entity: domain.Entity{ID: "G"}, Visibility: domain.VisibilityPublic}
	entity := domain.Entity{ID: "E", ProjectID: "G"}
	registry, err := generators.NewRegistry(mermaid.NewRenderer())
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"model", "collection", "field"} {
		t.Run(kind, func(t *testing.T) {
			source := &unavailableSelection{}
			h := NewHandler(testWeaveStore{project: project, model: domain.Model{Entity: entity}, collection: domain.Collection{Entity: entity}, field: domain.Field{Entity: entity}}, generators.NewService(source, registry), nil, nil)
			router := chi.NewRouter()
			router.Get("/model/{modelID}", h.GetModelDiagram)
			router.Get("/collection/{collectionID}", h.GetCollectionDiagram)
			router.Get("/field/{fieldID}", h.GetFieldDiagram)
			req := superAdminRequest(http.MethodGet, "/"+kind+"/E")
			req = req.WithContext(auth.WithProjectVersion(req.Context(), "1.0"))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "not_in_release") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if source.calls != 1 || !source.scope.IsRelease() || source.scope.Version() != "1.0" {
				t.Fatalf("selection changed or retried: calls=%d scope=%+v", source.calls, source.scope)
			}
		})
	}
}
