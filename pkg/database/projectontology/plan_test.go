package projectontology

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/pletka-io/pletka/pkg/ontologyartifact"
)

type fixtureReader struct {
	candidates []Candidate
	artifacts  map[string]ontologyartifact.Artifact
	failure    error
}

func (r fixtureReader) Candidates(context.Context) ([]Candidate, error) {
	return append([]Candidate{}, r.candidates...), r.failure
}
func (r fixtureReader) Publication(_ context.Context, version string) (ontologyartifact.Artifact, error) {
	if r.failure != nil {
		return ontologyartifact.Artifact{}, r.failure
	}
	a, ok := r.artifacts[version]
	if !ok {
		return a, &InvalidError{Detail: "unpublished"}
	}
	return a, nil
}
func publication(t *testing.T, owner, version string, pins []ontologyartifact.Pin) ontologyartifact.Artifact {
	t.Helper()
	body := fmt.Sprintf(`{"format_version":1,"ontology":{"id":%q},"version":{"id":%q,"ontology_id":%q}}`, owner, version, owner)
	hash := func(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
	raw, err := json.Marshal(ontologyartifact.Document{FormatVersion: 1, OntologyID: owner, VersionID: version, BodyJSON: body, BodyDigest: hash(body), Pins: pins, Requirements: []ontologyartifact.Requirement{}})
	if err != nil {
		t.Fatal(err)
	}
	return ontologyartifact.Artifact{JSON: string(raw), Digest: hash(string(raw))}
}
func TestProjectPlanDefaultsExplicitPinsAndSourceConflicts(t *testing.T) {
	d1, d2 := publication(t, "defaults", "d1", nil), publication(t, "defaults", "d2", nil)
	pin := ontologyartifact.Pin{OntologyID: "defaults", VersionID: "d1", Digest: d1.Digest}
	own := publication(t, "own", "o1", []ontologyartifact.Pin{pin})
	r := fixtureReader{candidates: []Candidate{{OntologyID: "defaults", Prefix: "defaults", VersionID: "d1", Published: true}, {OntologyID: "defaults", Prefix: "defaults", VersionID: "d2", Published: true, Active: true}}, artifacts: map[string]ontologyartifact.Artifact{"d1": d1, "d2": d2, "o1": own}}
	if _, err := Plan(t.Context(), r, []string{"o1"}, nil, true); err == nil {
		t.Fatal("inherited closure silently overrode active defaults")
	}
	got, err := Plan(t.Context(), r, []string{"o1", "d1"}, nil, true)
	if err != nil || len(got) != 2 {
		t.Fatalf("explicit defaults did not override active policy: %v", err)
	}
	if _, err := Plan(t.Context(), r, []string{"d1", "d2"}, nil, false); err == nil {
		t.Fatal("two direct revisions accepted")
	}
	if _, err := Plan(t.Context(), r, []string{"d2"}, []ontologyartifact.Pin{pin}, false); err == nil {
		t.Fatal("source revision conflict accepted")
	}
	failure := errors.New("database read failed")
	r.failure = failure
	if _, err := Plan(t.Context(), r, []string{"o1"}, nil, true); !errors.Is(err, failure) {
		t.Fatalf("transport failure hidden: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r.failure = nil
	if _, err := Plan(ctx, r, []string{"o1"}, nil, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
