package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

type fakeModels struct {
	models  []*domain.Model
	gotOpts []domain.QueryOption
}

// List emulates the store's SQL-backed status filter (Task 1): a non-empty
// cfg.Filters["status"] narrows both the returned rows and the count.
func (f *fakeModels) List(_ context.Context, _ string, opts ...domain.QueryOption) ([]*domain.Model, int64, error) {
	f.gotOpts = opts
	cfg := domain.ApplyOptions(opts)
	status, _ := cfg.Filters["status"].(string)
	if status == "" {
		return f.models, int64(len(f.models)), nil
	}
	var matched []*domain.Model
	for _, m := range f.models {
		if string(m.Status) == status {
			matched = append(matched, m)
		}
	}
	return matched, int64(len(matched)), nil
}
func (f *fakeModels) Get(_ context.Context, _, id string) (*domain.Model, error) {
	for _, m := range f.models {
		if m.ID == id {
			return m, nil
		}
	}
	return nil, errors.New("not found")
}

// modelWith builds a domain.Model with the fields the entity tools tests
// need. status is converted to domain.Status explicitly — Entity.Status is
// a defined string type, so a plain string variable does not implicitly
// convert (see domain.Status doc comment).
func modelWith(id, semantic, status string) *domain.Model {
	m := &domain.Model{}
	m.ID = id
	m.SemanticID = semantic
	m.Status = domain.Status(status)
	return m
}

func testHostEntities() Host {
	h := testHostProjects() // from tools_projects_test.go — readable project LA
	h.Models = &fakeModels{models: []*domain.Model{
		modelWith("01ULIDAAA", "LAM.1", "published"),
		modelWith("01ULIDBBB", "LAM.2", "draft"),
	}}
	return h
}

func TestListEntitiesUnknownType(t *testing.T) {
	h := testHostEntities()
	if _, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "widget"}); err == nil {
		t.Fatal("unknown entity_type must error")
	}
}

func TestListEntitiesStatusFilter(t *testing.T) {
	h := testHostEntities()
	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "model", Status: "draft"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Entities) != 1 || out.Entities[0].SemanticID != "LAM.2" {
		t.Fatalf("want only LAM.2, got %+v", out.Entities)
	}
	if out.TotalCount != 1 {
		t.Fatalf("want TotalCount 1 (post-filter match count), got %d", out.TotalCount)
	}
}

func TestListEntitiesBaseClass(t *testing.T) {
	h := testHostEntities()
	scoped := modelWith("01ULIDCCC", "LAM.3", "published")
	scoped.OntologyScope = domain.PathElement{Type: "class", Prefix: "crm", LocalName: "E22_Human-Made_Object"}
	h.Models = &fakeModels{models: []*domain.Model{scoped, modelWith("01ULIDDDD", "LAM.4", "published")}}

	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "model"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Entities) != 2 {
		t.Fatalf("want 2 entities, got %d", len(out.Entities))
	}
	if out.Entities[0].BaseClass != "crm:E22_Human-Made_Object" {
		t.Fatalf("want BaseClass crm:E22_Human-Made_Object, got %q", out.Entities[0].BaseClass)
	}
	if out.Entities[1].BaseClass != "" {
		t.Fatalf("want empty BaseClass for zero-value OntologyScope, got %q", out.Entities[1].BaseClass)
	}
}

type fakeFields struct {
	fields []*domain.Field
}

func (f *fakeFields) List(_ context.Context, _ string, _ ...domain.QueryOption) ([]*domain.Field, int64, error) {
	return f.fields, int64(len(f.fields)), nil
}
func (f *fakeFields) GetByIdentifier(_ context.Context, _, id string) (*domain.Field, error) {
	for _, fld := range f.fields {
		if fld.ID == id {
			return fld, nil
		}
	}
	return nil, errors.New("not found")
}

func TestListEntitiesFieldPathElements(t *testing.T) {
	h := testHostEntities()
	fld := &domain.Field{}
	fld.ID = "01ULIDFFF"
	fld.SemanticID = "LAF.1"
	fld.Status = domain.Status("published")
	fld.PathElements = []domain.PathElement{
		{Type: "class", Prefix: "crm", LocalName: "E21_Person"},
		{Type: "property", Prefix: "crm", LocalName: "P1_is_identified_by"},
	}
	h.Fields = &fakeFields{fields: []*domain.Field{fld}}

	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "field"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Entities) != 1 {
		t.Fatalf("want 1 entity, got %d", len(out.Entities))
	}
	if len(out.Entities[0].PathElements) != 2 {
		t.Fatalf("want 2 path elements, got %d", len(out.Entities[0].PathElements))
	}
	if out.Entities[0].PathElements[0].LocalName != "E21_Person" {
		t.Fatalf("want first path element LocalName E21_Person, got %q", out.Entities[0].PathElements[0].LocalName)
	}
}

type fakeCategories struct {
	categories []*domain.Category
}

// List ignores opts entirely — the category store does not support paging
// or filters (see task brief); the tool compensates in memory.
func (f *fakeCategories) List(_ context.Context, _ string, _ ...domain.QueryOption) ([]*domain.Category, error) {
	return f.categories, nil
}
func (f *fakeCategories) Get(_ context.Context, _, id string) (*domain.Category, error) {
	for _, c := range f.categories {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, errors.New("not found")
}

func categoryWith(id, semantic, status string) *domain.Category {
	c := &domain.Category{}
	c.ID = id
	c.SemanticID = semantic
	c.Status = domain.Status(status)
	return c
}

func TestListEntitiesCategoryFilterBeforePaging(t *testing.T) {
	h := testHostEntities()
	h.Categories = &fakeCategories{categories: []*domain.Category{
		categoryWith("01C1", "LA.CAT.1", "published"),
		categoryWith("01C2", "LA.CAT.2", "draft"),
		categoryWith("01C3", "LA.CAT.3", "published"),
		categoryWith("01C4", "LA.CAT.4", "draft"),
		categoryWith("01C5", "LA.CAT.5", "published"),
	}}

	out, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "category", Status: "published", Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Entities) != 2 {
		t.Fatalf("want 2 rows, got %d", len(out.Entities))
	}
	if out.TotalCount != 3 {
		t.Fatalf("want TotalCount 3 (all published), got %d", out.TotalCount)
	}

	out2, err := listEntities(context.Background(), h, listEntitiesInput{ProjectID: "LA", EntityType: "category", Status: "published", Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("list offset: %v", err)
	}
	if len(out2.Entities) != 1 {
		t.Fatalf("want 1 row at offset 2, got %d", len(out2.Entities))
	}
	if out2.TotalCount != 3 {
		t.Fatalf("want TotalCount 3, got %d", out2.TotalCount)
	}
}

func TestGetEntitySemanticFallback(t *testing.T) {
	h := testHostEntities()
	out, err := getEntity(context.Background(), h, getEntityInput{ProjectID: "LA", EntityType: "model", ID: "LAM.2"})
	if err != nil {
		t.Fatalf("get by semantic id: %v", err)
	}
	m, ok := out.Entity.(*domain.Model)
	if !ok || m.SemanticID != "LAM.2" {
		t.Fatalf("want LAM.2 model, got %#v", out.Entity)
	}

	// Verify the fallback scan includes the limit option
	fake := h.Models.(*fakeModels)
	if len(fake.gotOpts) == 0 {
		t.Fatal("expected at least one query option (limit)")
	}
	cfg := domain.ApplyOptions(fake.gotOpts)
	if cfg.Limit != 10000 {
		t.Fatalf("expected limit %d, got %d", 10000, cfg.Limit)
	}
}
