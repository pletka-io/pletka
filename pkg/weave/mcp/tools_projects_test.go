package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

type fakeProjects struct {
	projects map[string]*domain.Project
	readable map[string]bool
}

func (f *fakeProjects) ListVisible(_ context.Context, _ ...domain.QueryOption) ([]*domain.Project, int64, error) {
	var out []*domain.Project
	for id, p := range f.projects {
		if f.readable[id] {
			out = append(out, p)
		}
	}
	return out, int64(len(out)), nil
}
func (f *fakeProjects) Get(_ context.Context, id string) (*domain.Project, error) {
	p, ok := f.projects[id]
	if !ok {
		// domain has no ErrNotFound (verified: grep -rn "ErrNotFound" pkg/domain/ is empty).
		return nil, errors.New("not found")
	}
	return p, nil
}
func (f *fakeProjects) CanRead(_ context.Context, p *domain.Project) bool { return f.readable[p.ID] }
func (f *fakeProjects) StatsForProjects(_ context.Context, ids []string) (map[string]*domain.WeaveProjectStats, error) {
	out := map[string]*domain.WeaveProjectStats{}
	for _, id := range ids {
		out[id] = &domain.WeaveProjectStats{FieldCount: 3}
	}
	return out, nil
}

type fakeOntLinks struct{}

func (fakeOntLinks) LinkedOntologies(_ context.Context, _ string) ([]domain.LinkedOntology, error) {
	return []domain.LinkedOntology{{Name: "CIDOC-CRM", Version: "7.1.3"}}, nil
}

func testHostProjects() Host {
	la := &domain.Project{}
	la.ID = "LA"
	hidden := &domain.Project{}
	hidden.ID = "SEC"
	return Host{
		Projects: &fakeProjects{
			projects: map[string]*domain.Project{"LA": la, "SEC": hidden},
			readable: map[string]bool{"LA": true},
		},
		Ontologies: fakeOntLinks{},
	}
}

func TestListProjectsVisibleOnly(t *testing.T) {
	h := testHostProjects()
	out, err := listProjects(context.Background(), h)
	if err != nil {
		t.Fatalf("list_projects: %v", err)
	}
	if len(out.Projects) != 1 || out.Projects[0].ID != "LA" {
		t.Fatalf("want only LA, got %+v", out.Projects)
	}
	if out.Projects[0].FieldCount != 3 {
		t.Fatal("stats not attached")
	}
}

func TestGetProjectDeniedIsNotFound(t *testing.T) {
	h := testHostProjects()
	if _, err := getProject(context.Background(), h, "SEC"); err == nil {
		t.Fatal("unreadable project must error (indistinguishable from missing)")
	}
	out, err := getProject(context.Background(), h, "LA")
	if err != nil {
		t.Fatalf("get_project LA: %v", err)
	}
	if len(out.LinkedOntologies) != 1 {
		t.Fatal("linked ontologies missing")
	}
}
