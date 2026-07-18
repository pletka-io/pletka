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

func (f *fakeModels) List(_ context.Context, _ string, opts ...domain.QueryOption) ([]*domain.Model, int64, error) {
	f.gotOpts = opts
	return f.models, int64(len(f.models)), nil
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
