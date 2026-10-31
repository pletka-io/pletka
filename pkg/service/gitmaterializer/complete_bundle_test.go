package gitmaterializer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

func completeBundleFixture(t *testing.T) string {
	t.Helper()
	id, err := projectsnapshot.Draft("project")
	if err != nil {
		t.Fatal(err)
	}
	c := projectsnapshot.Content{}
	v := reflect.ValueOf(&c).Elem()
	for i := range v.NumField() {
		v.Field(i).Set(reflect.MakeSlice(v.Field(i).Type(), 0, 0))
	}
	namespace, enforce := "https://example.test/", false
	c.Projects = []projectsnapshot.ProjectRecord{{ID: "project", OwnerID: "owner", EffectiveConceptNamespace: &namespace, EnforceConceptLists: &enforce}}
	counts := map[string]int64{}
	for i := range v.NumField() {
		counts[v.Type().Field(i).Tag.Get("json")] = int64(v.Field(i).Len())
	}
	descriptorJSON, err := json.Marshal(releasedescriptor.Document{Format: 1, ProjectID: "project", Actors: map[string]releasedescriptor.Actor{"owner": {ActorID: "owner"}}, ProjectModules: map[string]string{"project": "example.test/project"}, OntologyModules: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	descriptors, err := releasedescriptor.Decode(descriptorJSON, "project", "")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := projectsnapshot.DraftEvidence(id, descriptors, []ontologyartifact.ProjectArtifact{}, []ontologyartifact.TermResolution{})
	if err != nil {
		t.Fatal(err)
	}
	value, err := projectsnapshot.NewComplete(projectsnapshot.ProjectInput{Content: c, Evidence: evidence, Counts: counts, Dependencies: []projectsnapshot.Identity{}, Membership: []projectsnapshot.Membership{}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	files, err := projectbundle.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for path, data := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLoadCompleteProjectBundleSeparatesCatalogAndLegacy(t *testing.T) {
	root := completeBundleFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "releases"), 0o755); err != nil {
		t.Fatal(err)
	}
	catalog := []byte("releases: []\n")
	if err := os.WriteFile(filepath.Join(root, releasesIndexPath), catalog, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "ignored"), []byte("not snapshot data"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCompleteProjectBundle(root, projectbundle.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	id, err := loaded.Project.Snapshot().Root()
	if err != nil || id.ProjectID() != "project" || !id.IsDraft() {
		t.Fatalf("wrong imported value: %v", err)
	}
	if string(loaded.Catalog) != string(catalog) {
		t.Fatal("catalog changed")
	}
	if _, err := LoadProjectSnapshot(root); err == nil || !strings.Contains(err.Error(), "unsupported legacy project schema 3") {
		t.Fatalf("legacy loader did not reject complete format first: %v", err)
	}
	legacy := loadSnapshotFixture(t)
	if _, err := LoadCompleteProjectBundle(legacy.RootDir, projectbundle.DefaultLimits()); err == nil {
		t.Fatal("old partial tree acquired completeness")
	}
}

func TestCompleteBundleFileSafetyAndLimits(t *testing.T) {
	for _, name := range []string{"symlink", "unknown file", "file budget", "byte budget"} {
		t.Run(name, func(t *testing.T) {
			root := completeBundleFixture(t)
			limits := projectbundle.DefaultLimits()
			switch name {
			case "symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(root, "escape")); err != nil {
					t.Fatal(err)
				}
			case "unknown file":
				if err := os.WriteFile(filepath.Join(root, "unexpected"), []byte("extra"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "file budget":
				limits.MaxFiles = 1
			case "byte budget":
				limits.MaxBytes = 1
			}
			if _, err := LoadCompleteProjectBundle(root, limits); err == nil {
				t.Fatal("accepted unsafe or excessive tree")
			}
		})
	}
}
