package ontologyartifact_test

import (
	"os"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/ontologyartifact"
)

func TestExactPublicationBytesAndDetachedValues(t *testing.T) {
	raw, err := os.ReadFile("testdata/publication.json")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := os.ReadFile("testdata/publication.sha256")
	if err != nil {
		t.Fatal(err)
	}
	a := ontologyartifact.Artifact{JSON: string(raw), Digest: strings.TrimSpace(string(hash))}
	doc, err := a.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if doc.VersionID != "v1" || !strings.Contains(doc.BodyJSON, `"retained_unknown_field": [1, 2]`) {
		t.Fatal("exact body or stable version lost")
	}
	doc.Pins[0].VersionID = "changed"
	doc.Requirements[0].Name = "changed"
	content, err := a.Content()
	if err != nil {
		t.Fatal(err)
	}
	if len(content.Bindings) != 2 {
		t.Fatal("alias registry was collapsed")
	}
	registry, err := a.Namespaces()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Effective(); err == nil {
		t.Fatal("tied aliases became a checked effective namespace map")
	}
	content.Bindings[0].Namespace = "changed"
	doc, err = a.Decode()
	if err != nil || doc.Pins[0].VersionID != "s1" || doc.Requirements[0].Name != "source" {
		t.Fatalf("decoded values alias one another: %+v %v", doc, err)
	}
	content, err = a.Content()
	if err != nil || content.Bindings[0].Namespace != "https://one.example/" || content.Version.RDF != "<original/>" {
		t.Fatalf("content aliases another projection: %+v %v", content, err)
	}
	if a.JSON != string(raw) || a.Digest != strings.TrimSpace(string(hash)) {
		t.Fatal("portable artifact bytes or digest changed")
	}
	a.JSON += " "
	if _, err := a.Decode(); err == nil {
		t.Fatal("corrupted portable bytes accepted")
	}
	if _, err := (ontologyartifact.Artifact{}).Decode(); err == nil {
		t.Fatal("zero artifact accepted")
	}
}
