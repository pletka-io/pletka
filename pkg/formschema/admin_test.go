package formschema_test

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/formschema"
)

func TestBuildAdminSchema_SuperAdminSeesSections(t *testing.T) {
	got := formschema.BuildAdminSchema(&auth.AuthSnapshot{IsSuperAdmin: true})
	if got == nil {
		t.Fatal("nil schema")
	}
	if len(got.Sections) == 0 {
		t.Fatal("expected admin sections for superadmin")
	}
	if got.Sections[0].ID != "users" {
		t.Fatalf("first section = %q, want users", got.Sections[0].ID)
	}
	if got.Sections[0].SchemaURL != "/admin/users/entity-list-schema" {
		t.Fatalf("users schema URL = %q", got.Sections[0].SchemaURL)
	}
	if got.Sections[0].Kind != "entity-list" {
		t.Fatalf("users kind = %q", got.Sections[0].Kind)
	}
	foundNamespaces := false
	foundOntologies := false
	foundUsers := false
	foundInstitutions := false
	foundGitRestore := false
	foundVocabularies := false
	for _, section := range got.Sections {
		switch section.ID {
		case "users":
			foundUsers = true
			if section.SchemaURL != "/admin/users/entity-list-schema" {
				t.Fatalf("users schema URL = %q", section.SchemaURL)
			}
			if section.Kind != "entity-list" {
				t.Fatalf("users kind = %q", section.Kind)
			}
		case "institutions":
			foundInstitutions = true
			if section.SchemaURL != "/admin/institutions/entity-list-schema" {
				t.Fatalf("institutions schema URL = %q", section.SchemaURL)
			}
			if section.Kind != "entity-list" {
				t.Fatalf("institutions kind = %q", section.Kind)
			}
		case "global-namespaces":
			foundNamespaces = true
			if section.SchemaURL != "/admin/namespaces/entity-list-schema" {
				t.Fatalf("global namespace schema URL = %q", section.SchemaURL)
			}
			if section.Kind != "entity-list" {
				t.Fatalf("global namespace kind = %q", section.Kind)
			}
			if section.Placeholder {
				t.Fatal("global namespaces should not be a placeholder")
			}
		case "ontologies":
			foundOntologies = true
			if section.SchemaURL != "/admin/ontologies/entity-list-schema" {
				t.Fatalf("ontology schema URL = %q", section.SchemaURL)
			}
			if section.Kind != "entity-list" {
				t.Fatalf("ontology kind = %q", section.Kind)
			}
		case "git-restore":
			foundGitRestore = true
			if section.Kind != "git-restore" {
				t.Fatalf("git restore kind = %q", section.Kind)
			}
			if section.Placeholder {
				t.Fatal("git restore should not be a placeholder")
			}
		case "vocabularies":
			foundVocabularies = true
			if section.SchemaURL != "/admin/vocabularies/entity-list-schema" {
				t.Fatalf("vocabularies schema URL = %q", section.SchemaURL)
			}
			if section.Kind != "entity-list" {
				t.Fatalf("vocabularies kind = %q", section.Kind)
			}
		}
	}
	if !foundNamespaces {
		t.Fatal("global-namespaces section missing")
	}
	if !foundOntologies {
		t.Fatal("ontologies section missing")
	}
	if !foundUsers {
		t.Fatal("users section missing")
	}
	if !foundInstitutions {
		t.Fatal("institutions section missing")
	}
	if !foundGitRestore {
		t.Fatal("git-restore section missing")
	}
	if !foundVocabularies {
		t.Fatal("vocabularies section missing")
	}
}

func TestBuildAdminSchema_NonSuperAdminSeesNoSections(t *testing.T) {
	got := formschema.BuildAdminSchema(&auth.AuthSnapshot{})
	if got == nil {
		t.Fatal("nil schema")
	}
	if len(got.Sections) != 0 {
		t.Fatalf("sections = %v, want none", got.Sections)
	}
}

func TestBuildAdminSchema_AppendsHostSections(t *testing.T) {
	got := formschema.BuildAdminSchema(&auth.AuthSnapshot{IsSuperAdmin: true}, formschema.AdminSection{
		ID:   "platform-section",
		Kind: "external",
		Href: "/admin/platform",
	})
	if got == nil {
		t.Fatal("nil schema")
	}
	if len(got.Sections) == 0 {
		t.Fatal("expected sections")
	}
	last := got.Sections[len(got.Sections)-1]
	if last.ID != "platform-section" {
		t.Fatalf("last section = %q, want platform-section", last.ID)
	}
	if last.Href != "/admin/platform" {
		t.Fatalf("host section href = %q", last.Href)
	}
}
