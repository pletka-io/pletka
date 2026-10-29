package ontologysnapshot

import (
	"encoding/json"
	"errors"
	"testing"
)

func marshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func artifact(t *testing.T, d Document) Artifact {
	t.Helper()
	s := marshal(t, d)
	return Artifact{JSON: s, Digest: hash(s)}
}

func revision(t *testing.T, id string) Body {
	t.Helper()
	s := marshal(t, map[string]any{
		"format_version": 1, "ontology": map[string]string{"id": "crm"},
		"version": map[string]string{"id": id, "ontology_id": "crm", "version_string": "7.1.3"},
		"classes": []map[string]string{{"uri": "https://example.org/E1", "prefix": "crm", "local_name": "E1"}},
	})
	return Body{OntologyID: "crm", VersionID: id, Digest: hash(s), BodyJSON: s,
		Selection: json.RawMessage(`{"project_id":"legacy-project","version_number":"1","ontology_version_id":"` + id + `","is_primary":true}`)}
}

func valueFixture(t *testing.T) Document {
	t.Helper()
	d := Document{FormatVersion: 1, ProjectID: "legacy-project", Version: "1", Inventory: []Body{revision(t, "revision-a"), revision(t, "revision-b")},
		Elements: []Element{{EntityKind: "field", EntityID: "f", Location: "/path_elements/0", Original: json.RawMessage(`{"type":"class","prefix":"crm","local_name":"E1"}`)}}}
	bodies := make([]bodyDocument, 0, len(d.Inventory))
	for _, b := range d.Inventory {
		body, err := decodeBody(b)
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, body)
	}
	refs, err := elementReferences(d.Elements[0])
	if err != nil {
		t.Fatal(err)
	}
	refs[0].Status, refs[0].Candidates = assess(refs[0].Reference, d.Inventory, bodies, nil)
	d.Assessments = refs
	return d
}

func TestDistinctRevisionsWithSameUpstreamLabelSurvive(t *testing.T) {
	d := valueFixture(t)
	a := artifact(t, d)
	got, err := a.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Inventory) != 2 || got.Assessments[0].Status != Ambiguous || len(got.Assessments[0].Candidates) != 2 {
		t.Fatalf("lost revision evidence: %+v", got)
	}
	_, err = a.Resolve("field", "f", "/path_elements/0")
	var unresolved *UnresolvedError
	if !errors.As(err, &unresolved) || unresolved.Status != Ambiguous {
		t.Fatalf("ambiguity became a definition: %v", err)
	}
	if _, _, err := (Definition{}).Evidence(); err == nil {
		t.Fatal("zero definition usable")
	}
}

func TestDetachedSnapshotRejectsForgedEvidence(t *testing.T) {
	for name, mutate := range map[string]func(*Document){
		"format":             func(d *Document) { d.FormatVersion = 2 },
		"missing assessment": func(d *Document) { d.Assessments = nil },
		"promote ambiguity": func(d *Document) {
			d.Assessments[0].Status = Resolved
			d.Assessments[0].Candidates = d.Assessments[0].Candidates[:1]
		},
		"changed original":   func(d *Document) { d.Elements[0].Original = json.RawMessage(`{"type":"class","uri":"unknown:A"}`) },
		"wrong owner":        func(d *Document) { d.Inventory[0].OntologyID = "other" },
		"changed body":       func(d *Document) { d.Inventory[0].BodyJSON += " " },
		"duplicate identity": func(d *Document) { d.Inventory[1] = d.Inventory[0] },
		"escaped selection":  func(d *Document) { d.Inventory[0].Selection = json.RawMessage(`{"project_id":"elsewhere"}`) },
		"escaped dependency": func(d *Document) {
			d.Dependencies = []Dependency{{ProjectID: "legacy-project", Version: "1", SourceProject: "elsewhere", SourceVersion: "1"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := valueFixture(t)
			mutate(&d)
			if _, err := artifact(t, d).Decode(); err == nil {
				t.Fatal("forged evidence accepted")
			}
		})
	}
}

func TestExplicitVersionAndMissingInvalidDistinctions(t *testing.T) {
	d := valueFixture(t)
	bodies := make([]bodyDocument, 0, len(d.Inventory))
	for _, b := range d.Inventory {
		body, err := decodeBody(b)
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, body)
	}
	for _, tc := range []struct {
		ref    TermReference
		status Status
	}{
		{TermReference{Kind: "class", URI: "https://example.org/E1", VersionID: "revision-a"}, Resolved},
		{TermReference{Kind: "class", URI: "https://example.org/E1", VersionID: "outside"}, Invalid},
		{TermReference{Kind: "class", URI: "rdf:literal"}, Missing},
		{TermReference{Kind: "class", Prefix: "crm"}, Invalid},
		{TermReference{Kind: "unsupported", URI: "crm:E1"}, Invalid},
		{TermReference{Kind: "property", URI: "crm:E1"}, Missing},
	} {
		status, _ := assess(tc.ref, d.Inventory, bodies, nil)
		if status != tc.status {
			t.Errorf("%+v: %s want %s", tc.ref, status, tc.status)
		}
	}
}

func TestSourceClosureRejectsMissingAndConflictingContextEvidence(t *testing.T) {
	for _, mode := range []string{"missing", "wrong digest", "nested", "conflicting body"} {
		t.Run(mode, func(t *testing.T) {
			root := valueFixture(t)
			source := valueFixture(t)
			source.ProjectID = "source"
			source.Elements = nil
			source.Assessments = nil
			for i := range source.Inventory {
				source.Inventory[i].Selection = json.RawMessage(marshal(t, map[string]string{"project_id": "source", "version_number": "1", "ontology_version_id": source.Inventory[i].VersionID}))
			}
			if mode == "conflicting body" {
				source.Inventory[0].BodyJSON += " "
				source.Inventory[0].Digest = hash(source.Inventory[0].BodyJSON)
			}
			if mode == "nested" {
				source.Sources = []Source{{JSON: "{}", Digest: hash("{}")}}
			}
			a := artifact(t, source)
			root.Sources = []Source{{JSON: a.JSON, Digest: a.Digest}}
			root.SourceRefs = []ContextReference{{ProjectID: "source", Version: "1", Digest: a.Digest}}
			if mode == "missing" {
				root.Sources = nil
			}
			if mode == "wrong digest" {
				root.SourceRefs[0].Digest = hash("wrong")
			}
			if _, err := artifact(t, root).Decode(); err == nil {
				t.Fatal("forged source closure accepted")
			}
		})
	}
}
