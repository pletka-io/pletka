package releasedescriptor_test

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

func TestDescriptorValuesOwnTheirData(t *testing.T) {
	raw := []byte(`{"format":1,"project_id":"project","version":"1.0.0","actors":{"owner":{"actor_id":"owner","display_name":"Original"}},"project_modules":{"project":"example.org/project"},"ontology_modules":{"v1":"example.org/ontology"}}`)
	s, err := releasedescriptor.Decode(raw, "project", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	clear(raw)
	id := "owner"
	actor, err := s.Actor(&id)
	if err != nil {
		t.Fatal(err)
	}
	actor.DisplayName = "changed"
	modules := s.OntologyModules()
	modules["v1"] = "changed"
	actor, err = s.Actor(&id)
	if err != nil || actor.DisplayName != "Original" {
		t.Fatalf("shared actor: %+v, %v", actor, err)
	}
	module, err := s.OntologyModule("v1")
	if err != nil || module != "example.org/ontology" || s.Module() != "example.org/project" {
		t.Fatalf("shared module: %s, %v", module, err)
	}
}

func TestDescriptorIdentityCannotBeRetargeted(t *testing.T) {
	raw := []byte(`{"format":1,"project_id":"project","version":"1.0.0","actors":{},"project_modules":{"project":"example.org/project"},"ontology_modules":{}}`)
	for _, pair := range [][2]string{{"other", "1.0.0"}, {"project", "2.0.0"}, {"", ""}} {
		if _, err := releasedescriptor.Decode(raw, pair[0], pair[1]); err == nil {
			t.Fatalf("accepted wrong identity %v", pair)
		}
	}
	var zero releasedescriptor.Snapshot
	if _, err := zero.ProjectModule("project"); err == nil {
		t.Fatal("zero value supplied a module")
	}
}
