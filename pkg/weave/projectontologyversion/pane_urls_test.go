package projectontologyversion

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestDecoratePaneGroupsForEdit covers the in-place URL decoration
// applied to own pane groups when the caller has project.edit. This
// is the schema-driven contract surface read by LinkedOntologiesPanel
// in the frontend — every per-row URL assertion below maps to a
// button gate over there.
func TestDecoratePaneGroupsForEdit(t *testing.T) {
	groups := []PaneGroup{
		{
			BaseVersionID: "v1",
			Origin:        domain.OwnOrigin(),
			Base: &PaneOntology{
				VersionID: "v1",
			},
			Extensions: []PaneOntology{
				{VersionID: "ext-a"},
				{VersionID: "ext-b"},
			},
			AvailableExtensions: []PaneAvailable{
				{VersionID: "avail-a"},
			},
		},
	}

	decoratePaneGroupsForEdit("LA", groups)

	got := groups[0]
	wantBase := &PaneOntology{
		VersionID: "v1",
		UpdateURL: "/projects/LA/project-ontology-versions/v1",
		DeleteURL: "/projects/LA/project-ontology-versions/v1",
	}
	if diff := cmp.Diff(wantBase, got.Base); diff != "" {
		t.Errorf("base mismatch (-want +got):\n%s", diff)
	}

	wantExts := []PaneOntology{
		{
			VersionID: "ext-a",
			UpdateURL: "/projects/LA/project-ontology-versions/ext-a",
			DeleteURL: "/projects/LA/project-ontology-versions/ext-a",
		},
		{
			VersionID: "ext-b",
			UpdateURL: "/projects/LA/project-ontology-versions/ext-b",
			DeleteURL: "/projects/LA/project-ontology-versions/ext-b",
		},
	}
	if diff := cmp.Diff(wantExts, got.Extensions); diff != "" {
		t.Errorf("extensions mismatch (-want +got):\n%s", diff)
	}

	wantAvail := []PaneAvailable{
		{
			VersionID: "avail-a",
			EnableURL: "/projects/LA/project-ontology-versions",
		},
	}
	if diff := cmp.Diff(wantAvail, got.AvailableExtensions); diff != "" {
		t.Errorf("available_extensions mismatch (-want +got):\n%s", diff)
	}
}

// TestDecoratePaneGroupsForEdit_NilBase guards the orphan-extension
// path: own groups can have a nil Base when extensions exist but the
// base they extend isn't linked to this project. URLs still attach
// to the orphan extensions; the nil Base must not panic.
func TestDecoratePaneGroupsForEdit_NilBase(t *testing.T) {
	groups := []PaneGroup{
		{
			Origin: domain.OwnOrigin(),
			Base:   nil,
			Extensions: []PaneOntology{
				{VersionID: "orphan"},
			},
		},
	}

	decoratePaneGroupsForEdit("LA", groups)

	if got := groups[0].Extensions[0].DeleteURL; got != "/projects/LA/project-ontology-versions/orphan" {
		t.Errorf("orphan extension delete_url = %q; want suffix /orphan", got)
	}
}

// TestPaneURLHelpers locks the wire shape so a refactor that renames
// the route parameters surfaces here, not as a 404 in the browser.
func TestPaneURLHelpers(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"row", paneRowURL("LA", "v42"), "/projects/LA/project-ontology-versions/v42"},
		{"create", paneCreateURL("LA"), "/projects/LA/project-ontology-versions"},
		{"form-schema", paneFormSchemaURL("LA"), "/projects/LA/project-ontology-versions/form-schema?mode=create"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s URL = %q; want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

type paneTestProjectReader struct {
	projects map[string]*domain.Project
}

func (r *paneTestProjectReader) GetByID(_ context.Context, id string) (*domain.Project, error) {
	return r.projects[id], nil
}

func (r *paneTestProjectReader) ResolvedOntologyVersions(_ context.Context, _ string, _ domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error) {
	return nil, nil
}

type paneTestOntologyReader struct {
	items map[string]*domain.Ontology
}

func (r *paneTestOntologyReader) List(_ context.Context) ([]*domain.Ontology, error) { return nil, nil }
func (r *paneTestOntologyReader) GetByID(_ context.Context, id string) (*domain.Ontology, error) {
	return r.items[id], nil
}

type paneTestVersionReader struct {
	items map[string]*domain.OntologyVersion
}

func (r *paneTestVersionReader) GetByID(_ context.Context, id string) (*domain.OntologyVersion, error) {
	return r.items[id], nil
}

func (r *paneTestVersionReader) ListByOntology(_ context.Context, _ string) ([]*domain.OntologyVersion, error) {
	return nil, nil
}

func TestBuildInheritedPaneGroups_SplitsMultipleBaseFamiliesPerParent(t *testing.T) {
	svc := &Service{
		projects: &paneTestProjectReader{
			projects: map[string]*domain.Project{
				"LA": {
					Entity: domain.Entity{ID: "LA", UIName: domain.Translations{"en": "Linked Art"}},
				},
			},
		},
		ontology: &paneTestOntologyReader{
			items: map[string]*domain.Ontology{
				"crm":  {ID: "crm", Name: "CIDOC CRM", Prefix: "crm", OntologyType: domain.OntologyTypeBase},
				"aaao": {ID: "aaao", Name: "AAAo", Prefix: "aaao", OntologyType: domain.OntologyTypeExtension, ExtendsOntologyID: ptr("crm")},
				"la":   {ID: "la", Name: "Linked Art", Prefix: "la", OntologyType: domain.OntologyTypeBase},
			},
		},
		versions: &paneTestVersionReader{
			items: map[string]*domain.OntologyVersion{
				"crm-v":  {ID: "crm-v", OntologyID: "crm", VersionString: "7.1.3"},
				"aaao-v": {ID: "aaao-v", OntologyID: "aaao", VersionString: "2.0"},
				"la-v":   {ID: "la-v", OntologyID: "la", VersionString: "1.0"},
			},
		},
	}

	rows := []domain.ResolvedOntologyVersion{
		{Link: &domain.ProjectOntologyVersion{ProjectID: "LA", OntologyVersionID: "crm-v", IsPrimary: true}, SourceProjectID: "LA", Origin: domain.InheritedOrigin("LA")},
		{Link: &domain.ProjectOntologyVersion{ProjectID: "LA", OntologyVersionID: "aaao-v"}, SourceProjectID: "LA", Origin: domain.InheritedOrigin("LA")},
		{Link: &domain.ProjectOntologyVersion{ProjectID: "LA", OntologyVersionID: "la-v"}, SourceProjectID: "LA", Origin: domain.InheritedOrigin("LA")},
	}

	groups, err := svc.buildInheritedPaneGroups(context.Background(), rows)
	if err != nil {
		t.Fatalf("buildInheritedPaneGroups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups len=%d want 2", len(groups))
	}
	if groups[0].Base == nil || groups[0].Base.OntologyID != "crm" {
		t.Fatalf("group[0] base ontology=%v want crm", groups[0].Base)
	}
	if len(groups[0].Extensions) != 1 || groups[0].Extensions[0].OntologyID != "aaao" {
		t.Fatalf("group[0] extensions=%v want [aaao]", groups[0].Extensions)
	}
	if groups[1].Base == nil || groups[1].Base.OntologyID != "la" {
		t.Fatalf("group[1] base ontology=%v want la", groups[1].Base)
	}
	if len(groups[1].Extensions) != 0 {
		t.Fatalf("group[1] extensions len=%d want 0", len(groups[1].Extensions))
	}
	if groups[0].Origin.SourceProjectLabel != "Linked Art" || groups[1].Origin.SourceProjectLabel != "Linked Art" {
		t.Fatalf("origin labels not preserved: %#v / %#v", groups[0].Origin, groups[1].Origin)
	}
}

func ptr[T any](v T) *T { return &v }
