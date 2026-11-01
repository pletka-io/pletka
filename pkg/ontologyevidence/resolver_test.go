package ontologyevidence

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestResolverRetainsValidatedContextAndFailsClosed(t *testing.T) {
	d := valueFixture(t)
	// Resolve a concrete revision while preserving another ambiguous location.
	e := Element{EntityKind: "field", EntityID: "pinned", Location: "/path_elements/0", Original: json.RawMessage(`{"type":"class","prefix":"crm","local_name":"E1","ontology_version_id":"revision-a"}`)}
	d.Elements = append(d.Elements, e)
	d.Assessments = append(d.Assessments, Assessment{EntityKind: e.EntityKind, EntityID: e.EntityID, Location: e.Location, Reference: TermReference{Kind: "class", Prefix: "crm", LocalName: "E1", VersionID: "revision-a"}, Status: Resolved, Candidates: []Candidate{{OntologyID: "crm", VersionID: "revision-a", Digest: d.Inventory[0].Digest, URI: "https://example.org/E1"}}})
	a := artifact(t, d)
	r, err := a.Resolver()
	if err != nil {
		t.Fatal(err)
	}
	// Caller-owned values can change without changing the prepared capability.
	a.JSON = "{}"
	d.Assessments[1].Candidates[0].URI = "changed"
	for range 2 {
		def, err := r.Resolve("field", "pinned", "/path_elements/0")
		if err != nil {
			t.Fatal(err)
		}
		c, body, err := def.Evidence()
		if err != nil || c.VersionID != "revision-a" || c.URI != "https://example.org/E1" || body == "" {
			t.Fatalf("lost definition: %+v %v", c, err)
		}
	}
	for _, tc := range []struct {
		id     string
		status Status
	}{{"f", Ambiguous}, {"missing", Missing}} {
		_, err := r.Resolve("field", tc.id, "/path_elements/0")
		var unresolved *UnresolvedError
		if !errors.As(err, &unresolved) || unresolved.Status != tc.status {
			t.Fatalf("%s: %v", tc.id, err)
		}
	}
	if _, err := (Resolver{}).Resolve("field", "pinned", "/path_elements/0"); err == nil {
		t.Fatal("zero resolver accepted")
	}
	if _, err := a.Resolver(); err == nil {
		t.Fatal("forged artifact accepted")
	}
}

func TestRepeatedReferenceStillChecksEveryAssessment(t *testing.T) {
	d := valueFixture(t)
	e := d.Elements[0]
	e.EntityID = "second"
	d.Elements = append(d.Elements, e)
	a := d.Assessments[0]
	a.EntityID = e.EntityID
	a.Candidates = append([]Candidate(nil), a.Candidates...)
	d.Assessments = append(d.Assessments, a)
	if _, err := artifact(t, d).Decode(); err != nil {
		t.Fatal(err)
	}
	d.Assessments[1].Candidates[0].URI = "forged"
	if _, err := artifact(t, d).Decode(); err == nil {
		t.Fatal("repeated reference skipped a forged assessment")
	}
}
