package ontologyrelease

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func restoreOrderArtifact(t *testing.T, owner, version string, pins ...Pin) Artifact {
	t.Helper()
	body := bodyDocument{FormatVersion: 1}
	body.Ontology.ID = owner
	body.Version.ID = version
	body.Version.OntologyID = owner
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	doc := Document{FormatVersion: 1, OntologyID: owner, VersionID: version,
		BodyJSON: string(data), BodyDigest: digest(string(data)), Pins: pins}
	data, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return Artifact{JSON: string(data), Digest: digest(string(data))}
}

func TestOrderArtifactsIndependentOwnerVersions(t *testing.T) {
	firstSource := restoreOrderArtifact(t, "source", "z-source-1")
	secondSource := restoreOrderArtifact(t, "source", "z-source-2")
	first := restoreOrderArtifact(t, "root", "a-root-1", Pin{OntologyID: "source", VersionID: "z-source-1", Digest: firstSource.Digest})
	second := restoreOrderArtifact(t, "root", "b-root-2", Pin{OntologyID: "source", VersionID: "z-source-2", Digest: secondSource.Digest})
	want := []Artifact{firstSource, first, secondSource, second}
	for _, tc := range []struct {
		name  string
		input []Artifact
	}{
		{"roots first", []Artifact{first, second, firstSource, secondSource}},
		{"reversed", []Artifact{secondSource, firstSource, second, first}},
		{"interleaved", []Artifact{second, firstSource, first, secondSource}},
		{"exact duplicates", []Artifact{second, first, second, secondSource, firstSource, firstSource}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := orderArtifacts(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, want) {
				t.Fatal("artifact bytes, deduplication or deterministic dependency order changed")
			}
		})
	}
}

func TestOrderArtifactsRejectsConflictingIdentityAndPins(t *testing.T) {
	source := restoreOrderArtifact(t, "source", "source-1")
	pin := Pin{OntologyID: "source", VersionID: "source-1", Digest: source.Digest}
	root := restoreOrderArtifact(t, "root", "root-1", pin)
	// Semantically identical JSON is still different published bytes.
	different := Artifact{JSON: source.JSON + "\n", Digest: digest(source.JSON + "\n")}
	tampered := root
	tampered.JSON += " "
	for _, tc := range []struct {
		name, failure string
		input         []Artifact
	}{
		{"same ID different bytes", "conflicting artifact bytes", []Artifact{source, different}},
		{"same ID different owner", "conflicting artifact bytes", []Artifact{source, restoreOrderArtifact(t, "other", "source-1")}},
		{"tampered artifact", "digest mismatch", []Artifact{tampered}},
		{"pin digest mismatch", "dependency identity mismatch", []Artifact{source, restoreOrderArtifact(t, "root", "root-1", Pin{OntologyID: "source", VersionID: "source-1", Digest: strings.Repeat("0", 64)})}},
		{"pin owner mismatch", "dependency identity mismatch", []Artifact{source, restoreOrderArtifact(t, "root", "root-1", Pin{OntologyID: "other", VersionID: "source-1", Digest: source.Digest})}},
		{"two versions in one closure", "invalid, conflicting or unordered pins", []Artifact{restoreOrderArtifact(t, "root", "root-1", pin, Pin{OntologyID: "source", VersionID: "source-2", Digest: source.Digest})}},
		{"duplicate pin in one closure", "invalid, conflicting or unordered pins", []Artifact{restoreOrderArtifact(t, "root", "root-1", pin, pin)}},
		{"self owner pin", "invalid, conflicting or unordered pins", []Artifact{restoreOrderArtifact(t, "source", "source-2", pin)}},
		{"unordered pins", "invalid, conflicting or unordered pins", []Artifact{restoreOrderArtifact(t, "root", "root-1", pin, Pin{OntologyID: "earlier", VersionID: "earlier-1", Digest: source.Digest})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := orderArtifacts(tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.failure) {
				t.Fatalf("wanted %q, got %v", tc.failure, err)
			}
			if got != nil {
				t.Fatal("invalid bundle returned partial artifacts")
			}
		})
	}
}

func TestOrderArtifactsRejectsCyclicPins(t *testing.T) {
	// Closing a cycle changes the first artifact's digest, invalidating the
	// back edge. Such bundles must fail even before the cycle guard is reached.
	first := restoreOrderArtifact(t, "first", "first-1")
	second := restoreOrderArtifact(t, "second", "second-1", Pin{OntologyID: "first", VersionID: "first-1", Digest: first.Digest})
	first = restoreOrderArtifact(t, "first", "first-1", Pin{OntologyID: "second", VersionID: "second-1", Digest: second.Digest})
	if _, err := orderArtifacts([]Artifact{first, second}); err == nil {
		t.Fatal("cyclic dependency bundle accepted")
	}
}

func TestOrderArtifactsAllowsDependenciesAlreadyInStorage(t *testing.T) {
	source := restoreOrderArtifact(t, "source", "source-1")
	root := restoreOrderArtifact(t, "root", "root-1", Pin{OntologyID: "source", VersionID: "source-1", Digest: source.Digest})
	got, err := orderArtifacts([]Artifact{root})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []Artifact{root}) {
		t.Fatal("ordering must leave absent dependency validation to storage")
	}
}
