package ontologyevidence_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
	"github.com/pletka-io/pletka/pkg/ontologyevidence"
)

func TestPortableLegacyEvidencePreservesExactBytesAndAmbiguity(t *testing.T) {
	raw, err := os.ReadFile("testdata/legacy.json")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := os.ReadFile("testdata/legacy.sha256")
	if err != nil {
		t.Fatal(err)
	}
	a := ontologyevidence.Artifact{JSON: string(raw), Digest: strings.TrimSpace(string(digest))}
	// The old public API is a type alias, with no second validator or conversion.
	d, err := decodeCompatibility(a)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := a.Reference()
	if err != nil {
		t.Fatal(err)
	}
	if ref.Origin().ProjectID() != "legacy-project" || ref.Origin().Version() != "1" || ref.Digest() != a.Digest {
		t.Fatal("reference lost exact identity")
	}
	if len(d.Inventory) != 2 || d.Assessments[0].Status != ontologyevidence.Ambiguous {
		t.Fatal("mixed revision evidence was collapsed")
	}
	_, err = a.Resolve("field", "f", "/path_elements/0")
	var unresolved *ontologyevidence.UnresolvedError
	if !errors.As(err, &unresolved) || unresolved.Status != ontologyevidence.Ambiguous {
		t.Fatalf("ambiguous evidence became a definition: %v", err)
	}
	registry, err := a.Namespaces()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Effective(); err == nil {
		t.Fatal("tied aliases became an output map")
	}
	before, err := a.Decode()
	if err != nil {
		t.Fatal(err)
	}
	d.Inventory[0].Selection[0] = '!'
	d.Elements[0].Original[0] = '!'
	d.Bindings[0].Namespace = "mutated"
	d.Assessments[0].Candidates[0].URI = "mutated"
	after, err := a.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(before, after); diff != "" {
		t.Fatal(diff)
	}
	bundle, err := ontologyevidence.PrepareBundle([]ontologyevidence.Artifact{a})
	if err != nil {
		t.Fatal(err)
	}
	bodies := bundle.Bodies()
	selection := string(bodies[0].Selection)
	bodies[0].Selection[0] = '!'
	if string(bundle.Bodies()[0].Selection) != selection {
		t.Fatal("bundle shares selection buffers")
	}
	artifacts := bundle.Artifacts()
	if artifacts[0] != a || artifacts[0].JSON != string(raw) {
		t.Fatal("bundle normalized original bytes or digest")
	}
	artifacts[0] = ontologyevidence.Artifact{}
	if bundle.Artifacts()[0] != a {
		t.Fatal("bundle shares artifact slice")
	}
}

func decodeCompatibility(value ontologysnapshot.Artifact) (ontologysnapshot.Document, error) {
	return value.Decode()
}
