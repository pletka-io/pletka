package mcp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
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

// requireProjectResourceInContext reproduces the gate real slice services
// apply (namespacebinding.Service.requireProjectRead,
// projectontologyversion.Service.requireProjectRead,
// category.Service.requireProjectRead): read the project resource
// auth.WithProjectResource-style middleware would have attached, and refuse
// to serve if it isn't there or doesn't match. The MCP transport never runs
// that HTTP middleware, so resolveProject must attach it itself
// (auth.WithProject) before calling into gated services — these fakes fail
// unless that happened.
func requireProjectResourceInContext(ctx context.Context, projectID string) error {
	res := auth.ProjectResourceFromContext(ctx)
	if res.ID != projectID {
		return fmt.Errorf("no project resource in context for %q (got %q) — resolveProject must thread its enriched ctx to this call", projectID, res.ID)
	}
	return nil
}

type fakeOntLinks struct{}

func (fakeOntLinks) LinkedOntologies(ctx context.Context, projectID string) ([]domain.LinkedOntology, error) {
	if err := requireProjectResourceInContext(ctx, projectID); err != nil {
		return nil, err
	}
	return []domain.LinkedOntology{{Name: "CIDOC-CRM", Version: "7.1.3"}}, nil
}

type fakeNamespaces struct{}

func (fakeNamespaces) ListForProject(ctx context.Context, projectID string) ([]*domain.NamespaceBinding, error) {
	if err := requireProjectResourceInContext(ctx, projectID); err != nil {
		return nil, err
	}
	return []*domain.NamespaceBinding{
		{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
		{Prefix: "aaao", Namespace: "https://ontology.swissartresearch.net/aaao/"},
	}, nil
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
		Namespaces: fakeNamespaces{},
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

// capabilityGatedOntLinks and capabilityGatedNamespaces reproduce the real
// gate used by namespacebinding.Service.requireProjectRead and
// projectontologyversion.Service.requireProjectRead exactly: pull the
// snapshot and the project resource from ctx and run the same
// snap.Can(auth.ProjectRead, res, nil) check the production services run.
// Unlike fakeOntLinks/fakeNamespaces (a plain ID match), this also exercises
// AuthSnapshot.Can's role lookup — so it only succeeds if resolveProject
// attached a project resource carrying the real ID/OrgID/Visibility a
// non-superadmin actor's explicit project role can match against.
type capabilityGatedOntLinks struct{}

func (capabilityGatedOntLinks) LinkedOntologies(ctx context.Context, _ string) ([]domain.LinkedOntology, error) {
	if !auth.FromContext(ctx).Can(auth.ProjectRead, auth.ProjectResourceFromContext(ctx), nil) {
		return nil, errors.New("forbidden: project.read denied — project resource missing from context")
	}
	return []domain.LinkedOntology{{Name: "CIDOC-CRM", Version: "7.1.3"}}, nil
}

type capabilityGatedNamespaces struct{}

func (capabilityGatedNamespaces) ListForProject(ctx context.Context, _ string) ([]*domain.NamespaceBinding, error) {
	if !auth.FromContext(ctx).Can(auth.ProjectRead, auth.ProjectResourceFromContext(ctx), nil) {
		return nil, errors.New("forbidden: project.read denied — project resource missing from context")
	}
	return []*domain.NamespaceBinding{
		{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
	}, nil
}

// TestGetProjectNonSuperAdminThreadsProjectResourceContext is the regression
// test for the bug: the MCP transport never runs the HTTP
// auth.WithProjectResource middleware, so namespacebinding.Service.List and
// projectontologyversion.Service.LinkedOntologies (both gated via
// auth.ProjectResourceFromContext) used to fail closed for every
// non-superadmin caller — only superadmin's IsSuperAdmin bypass masked it.
// This actor is a plain viewer with an explicit project role, no
// superadmin bypass available, so it only passes if resolveProject's
// returned ctx actually carries the project.
func TestGetProjectNonSuperAdminThreadsProjectResourceContext(t *testing.T) {
	la := &domain.Project{}
	la.ID = "LA"
	la.OwnerID = "org1"
	la.Visibility = "private"

	h := Host{
		Projects: &fakeProjects{
			projects: map[string]*domain.Project{"LA": la},
			readable: map[string]bool{"LA": true},
		},
		Ontologies: capabilityGatedOntLinks{},
		Namespaces: capabilityGatedNamespaces{},
	}

	snap := &auth.AuthSnapshot{
		ActorID: "alice",
		Roles:   map[string]string{"project:LA": "viewer"},
		// IsSuperAdmin deliberately false — this is the actor class the
		// live bug report ("get_project namespaces/linked_ontologies error
		// for normal users") hit; superadmin's bypass in Can() would mask
		// the missing project resource.
	}
	ctx := auth.WithSnapshot(context.Background(), snap)

	out, err := getProject(ctx, h, "LA")
	if err != nil {
		t.Fatalf("get_project as non-superadmin viewer: %v", err)
	}
	if len(out.LinkedOntologies) != 1 {
		t.Fatal("linked_ontologies empty — project resource context not threaded to projectontologyversion.Service")
	}
	if len(out.Namespaces) != 1 {
		t.Fatal("namespaces empty — project resource context not threaded to namespacebinding.Service")
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
	if len(out.Namespaces) != 2 {
		t.Fatalf("want 2 namespaces, got %d: %+v", len(out.Namespaces), out.Namespaces)
	}
	if out.Namespaces["aaao"] != "https://ontology.swissartresearch.net/aaao/" {
		t.Fatalf("namespaces[aaao] = %q, want aaao ontology URI", out.Namespaces["aaao"])
	}
}
