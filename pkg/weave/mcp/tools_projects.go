package mcp

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/pletka-io/pletka/pkg/domain"
)

// projectSummary is one row of list_projects output.
type projectSummary struct {
	ID              string              `json:"id"`
	UIName          domain.Translations `json:"ui_name"`
	Description     domain.Translations `json:"description,omitempty"`
	Visibility      string              `json:"visibility,omitempty"`
	ParentProjectID string              `json:"parent_project_id,omitempty"`
	FieldCount      int64               `json:"field_count"`
	ModelCount      int64               `json:"model_count"`
	CollectionCount int64               `json:"collection_count"`
	CategoryCount   int64               `json:"category_count"`
}

// listProjectsOutput is the result of the list_projects tool.
type listProjectsOutput struct {
	Projects []projectSummary `json:"projects"`
}

// getProjectInput is the input of the get_project tool.
type getProjectInput struct {
	ProjectID string `json:"project_id" jsonschema:"project ID / prefix, e.g. LA"`
}

// getProjectOutput is the result of the get_project tool.
type getProjectOutput struct {
	Project          projectSummary          `json:"project"`
	LinkedOntologies []domain.LinkedOntology `json:"linked_ontologies"`
	// Namespaces maps prefix -> namespace URI, as used in ontology paths.
	Namespaces map[string]string `json:"namespaces,omitempty"`
}

// resolveProject loads a project and enforces read access; unreadable and
// missing are indistinguishable to the caller.
func resolveProject(ctx context.Context, h Host, projectID string) (*domain.Project, error) {
	p, err := h.Projects.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("project %q not found", projectID)
	}
	if !h.Projects.CanRead(ctx, p) {
		return nil, fmt.Errorf("project %q not found", projectID)
	}
	return p, nil
}

// summarize builds a projectSummary from a project and its stats.
//
// Deviation from brief: domain.Project.ParentProjectID is *string (nil for
// root projects), not string as the brief's projectSummary field implied —
// dereferenced here when non-nil.
func summarize(p *domain.Project, stats *domain.WeaveProjectStats) projectSummary {
	s := projectSummary{
		ID:          p.ID,
		UIName:      p.UIName,
		Description: p.Description,
		Visibility:  p.Visibility,
	}
	if p.ParentProjectID != nil {
		s.ParentProjectID = *p.ParentProjectID
	}
	if stats != nil {
		s.FieldCount = stats.FieldCount
		s.ModelCount = stats.ModelCount
		s.CollectionCount = stats.CollectionCount
		s.CategoryCount = stats.CategoryCount
	}
	return s
}

// listProjects returns every project visible to the caller, with entity counts.
func listProjects(ctx context.Context, h Host) (listProjectsOutput, error) {
	projects, _, err := h.Projects.ListVisible(ctx)
	if err != nil {
		return listProjectsOutput{}, fmt.Errorf("list projects: %w", err)
	}
	ids := make([]string, 0, len(projects))
	for _, p := range projects {
		ids = append(ids, p.ID)
	}
	stats, err := h.Projects.StatsForProjects(ctx, ids)
	if err != nil {
		return listProjectsOutput{}, fmt.Errorf("project stats: %w", err)
	}
	out := listProjectsOutput{Projects: make([]projectSummary, 0, len(projects))}
	for _, p := range projects {
		out.Projects = append(out.Projects, summarize(p, stats[p.ID]))
	}
	return out, nil
}

// getProject returns one project's detail: entity counts and linked ontology versions.
func getProject(ctx context.Context, h Host, projectID string) (getProjectOutput, error) {
	p, err := resolveProject(ctx, h, projectID)
	if err != nil {
		return getProjectOutput{}, err
	}
	stats, err := h.Projects.StatsForProjects(ctx, []string{p.ID})
	if err != nil {
		return getProjectOutput{}, fmt.Errorf("project stats: %w", err)
	}
	linked, err := h.Ontologies.LinkedOntologies(ctx, p.ID)
	if err != nil {
		return getProjectOutput{}, fmt.Errorf("linked ontologies: %w", err)
	}
	bindings, err := h.Namespaces.ListForProject(ctx, p.ID)
	if err != nil {
		return getProjectOutput{}, fmt.Errorf("namespaces: %w", err)
	}
	namespaces := make(map[string]string, len(bindings))
	for _, b := range bindings {
		if b.Prefix == "" {
			continue
		}
		namespaces[b.Prefix] = b.Namespace
	}
	return getProjectOutput{
		Project:          summarize(p, stats[p.ID]),
		LinkedOntologies: linked,
		Namespaces:       namespaces,
	}, nil
}

// registerProjectTools attaches project-surface tools to the MCP server.
func registerProjectTools(s *sdk.Server, h Host) {
	sdk.AddTool(s, &sdk.Tool{
		Name:        "list_projects",
		Description: "List all Pletka projects visible to you, with entity counts.",
	}, instrumented(h, "list_projects", func(ctx context.Context, req *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, listProjectsOutput, error) {
		out, err := listProjects(ctx, h)
		return nil, out, err
	}))
	sdk.AddTool(s, &sdk.Tool{
		Name:        "get_project",
		Description: "Get one project's detail: entity counts, linked ontology versions, and the prefix→namespace map used in ontology paths.",
	}, instrumented(h, "get_project", func(ctx context.Context, req *sdk.CallToolRequest, in getProjectInput) (*sdk.CallToolResult, getProjectOutput, error) {
		out, err := getProject(ctx, h, in.ProjectID)
		return nil, out, err
	}))
}
