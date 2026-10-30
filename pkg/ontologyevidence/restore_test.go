package ontologyevidence

import (
	"encoding/json"
	"strings"
	"testing"
)

func restoreContext(t *testing.T, project string) Document {
	t.Helper()
	d := valueFixture(t)
	d.ProjectID = project
	for i := range d.Inventory {
		d.Inventory[i].Selection = json.RawMessage(marshal(t, map[string]any{
			"project_id": project, "version_number": d.Version, "ontology_version_id": d.Inventory[i].VersionID, "is_primary": true,
		}))
	}
	return d
}

func TestBundleRejectsCrossArtifactSourceContextConflicts(t *testing.T) {
	source := artifact(t, restoreContext(t, "shared-source"))
	first := restoreContext(t, "first")
	first.Sources = []Source{{JSON: source.JSON, Digest: source.Digest}}
	first.SourceRefs = []ContextReference{{ProjectID: "shared-source", Version: "1", Digest: source.Digest}}
	second := restoreContext(t, "second")
	second.Sources = []Source{{JSON: source.JSON + "\n", Digest: hash(source.JSON + "\n")}}
	second.SourceRefs = []ContextReference{{ProjectID: "shared-source", Version: "1", Digest: second.Sources[0].Digest}}
	values := []Artifact{artifact(t, first), artifact(t, second)}
	for _, a := range values {
		if _, err := a.Decode(); err != nil {
			t.Fatal(err)
		}
	}
	if err := ValidateBundle(values); err == nil || !strings.Contains(err.Error(), "source contexts") {
		t.Fatalf("bundle preflight accepted conflicting source contexts: %v", err)
	}
}

func TestValidateBundleAcceptsSharedSourceContext(t *testing.T) {
	source := artifact(t, restoreContext(t, "shared-source"))
	root := restoreContext(t, "root")
	root.Sources = []Source{{JSON: source.JSON, Digest: source.Digest}}
	root.SourceRefs = []ContextReference{{ProjectID: "shared-source", Version: "1", Digest: source.Digest}}
	if err := ValidateBundle([]Artifact{artifact(t, root), source, source}); err != nil {
		t.Fatalf("valid shared context rejected: %v", err)
	}
}

func TestBundleRejectsRootAndEmbeddedContextDisagreement(t *testing.T) {
	source := restoreContext(t, "shared-source")
	embedded := artifact(t, source)
	root := restoreContext(t, "root")
	root.Sources = []Source{{JSON: embedded.JSON, Digest: embedded.Digest}}
	root.SourceRefs = []ContextReference{{ProjectID: source.ProjectID, Version: source.Version, Digest: embedded.Digest}}
	source.Elements, source.Assessments = nil, nil
	if err := ValidateBundle([]Artifact{artifact(t, root), artifact(t, source)}); err == nil || !strings.Contains(err.Error(), "local contexts") {
		t.Fatalf("root/source disagreement accepted: %v", err)
	}
}
