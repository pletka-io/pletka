package namespace

import "testing"

func TestNewDefaultManager(t *testing.T) {
	mgr := NewDefaultManager()

	tests := []struct {
		prefix string
		uri    string
	}{
		{"rdf", "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
		{"rdfs", "http://www.w3.org/2000/01/rdf-schema#"},
		{"owl", "http://www.w3.org/2002/07/owl#"},
		{"xsd", "http://www.w3.org/2001/XMLSchema#"},
		{"skos", "http://www.w3.org/2004/02/skos/core#"},
		{"dc", "http://purl.org/dc/elements/1.1/"},
		{"dcterms", "http://purl.org/dc/terms/"},
	}

	for _, tt := range tests {
		ns, err := mgr.GetWithPrefix(tt.prefix)
		if err != nil {
			t.Errorf("GetWithPrefix(%q): %v", tt.prefix, err)
			continue
		}
		if ns.URI != tt.uri {
			t.Errorf("GetWithPrefix(%q).URI = %q, want %q", tt.prefix, ns.URI, tt.uri)
		}
	}

	ns, err := mgr.GetWithBase("http://www.w3.org/1999/02/22-rdf-syntax-ns#")
	if err != nil {
		t.Fatalf("GetWithBase(rdf): %v", err)
	}
	if ns.Prefix != "rdf" {
		t.Errorf("GetWithBase(rdf): prefix = %q, want %q", ns.Prefix, "rdf")
	}
}

func TestStandardNamespacesReturnsCopy(t *testing.T) {
	first := StandardNamespaces()
	if len(first) == 0 {
		t.Fatal("StandardNamespaces() returned no bindings")
	}
	first[0].URI = "https://mutated.example/"

	second := StandardNamespaces()
	if second[0].URI == "https://mutated.example/" {
		t.Fatal("StandardNamespaces() exposed mutable package state")
	}
}
