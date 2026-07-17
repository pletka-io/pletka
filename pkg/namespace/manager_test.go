package namespace

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSplitURI(t *testing.T) {
	tests := []struct {
		uri       string
		wantBase  string
		wantLocal string
	}{
		{"http://www.w3.org/1999/02/22-rdf-syntax-ns#type", "http://www.w3.org/1999/02/22-rdf-syntax-ns#", "type"},
		{"http://www.cidoc-crm.org/cidoc-crm/E21_Person", "http://www.cidoc-crm.org/cidoc-crm/", "E21_Person"},
		{"https://linked.art/ns/terms/HumanMadeObject", "https://linked.art/ns/terms/", "HumanMadeObject"},
		{"http://www.opengis.net/ont/geosparql#Feature", "http://www.opengis.net/ont/geosparql#", "Feature"},
		{"E21_Person", "", "E21_Person"},
		{"", "", ""},
		{"http://www.w3.org/2002/07/owl#", "http://www.w3.org/2002/07/owl#", ""},
		{"http://www.cidoc-crm.org/cidoc-crm/", "http://www.cidoc-crm.org/cidoc-crm/", ""},
	}

	for _, tt := range tests {
		base, local := SplitURI(tt.uri)
		if base != tt.wantBase || local != tt.wantLocal {
			t.Errorf("SplitURI(%q) = (%q, %q), want (%q, %q)",
				tt.uri, base, local, tt.wantBase, tt.wantLocal)
		}
	}
}

func TestManager_PutAndGet(t *testing.T) {
	mgr := NewStore()

	ns, err := mgr.Put("crm", "http://www.cidoc-crm.org/cidoc-crm/", 10)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if ns.Prefix != "crm" || ns.URI != "http://www.cidoc-crm.org/cidoc-crm/" {
		t.Errorf("Put returned %+v", ns)
	}

	got, err := mgr.GetWithPrefix("crm")
	if err != nil {
		t.Fatalf("GetWithPrefix: %v", err)
	}
	if diff := cmp.Diff(ns, got); diff != "" {
		t.Errorf("GetWithPrefix mismatch (-want +got):\n%s", diff)
	}

	got, err = mgr.GetWithBase("http://www.cidoc-crm.org/cidoc-crm/")
	if err != nil {
		t.Fatalf("GetWithBase: %v", err)
	}
	if got.Prefix != "crm" {
		t.Errorf("GetWithBase: prefix = %q, want %q", got.Prefix, "crm")
	}

	_, err = mgr.GetWithPrefix("unknown")
	if !errors.Is(err, ErrNamespaceNotFound) {
		t.Errorf("GetWithPrefix(unknown): err = %v, want ErrNamespaceNotFound", err)
	}

	_, err = mgr.Put("", "http://example.org/", 0)
	if !errors.Is(err, ErrNamespaceNotValid) {
		t.Errorf("Put empty prefix: err = %v, want ErrNamespaceNotValid", err)
	}
}

func TestManager_Expand(t *testing.T) {
	mgr := NewStore()
	mgr.Put("crm", "http://www.cidoc-crm.org/cidoc-crm/", 10)
	mgr.Put("rdf", "http://www.w3.org/1999/02/22-rdf-syntax-ns#", 10)

	tests := []struct {
		input string
		want  string
	}{
		{"crm:E21_Person", "http://www.cidoc-crm.org/cidoc-crm/E21_Person"},
		{"rdf:type", "http://www.w3.org/1999/02/22-rdf-syntax-ns#type"},
		{"http://example.org/foo", "http://example.org/foo"},
	}

	for _, tt := range tests {
		got, err := mgr.Expand(tt.input)
		if err != nil {
			t.Errorf("Expand(%q): %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Expand(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}

	_, err := mgr.Expand("unknown:foo")
	if err == nil {
		t.Error("Expand(unknown:foo): expected error")
	}
}

func TestManager_GetSearchLabel(t *testing.T) {
	mgr := NewStore()
	mgr.Put("crm", "http://www.cidoc-crm.org/cidoc-crm/", 10)
	mgr.Put("rdf", "http://www.w3.org/1999/02/22-rdf-syntax-ns#", 10)

	tests := []struct {
		uri  string
		want string
	}{
		{"http://www.cidoc-crm.org/cidoc-crm/E21_Person", "crm_E21_Person"},
		{"http://www.w3.org/1999/02/22-rdf-syntax-ns#type", "rdf_type"},
	}

	for _, tt := range tests {
		got, err := mgr.GetSearchLabel(tt.uri)
		if err != nil {
			t.Errorf("GetSearchLabel(%q): %v", tt.uri, err)
			continue
		}
		if got != tt.want {
			t.Errorf("GetSearchLabel(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

func TestManager_WeightPriority(t *testing.T) {
	mgr := NewStore()
	mgr.Put("dc", "http://purl.org/dc/elements/1.1/", 5)
	mgr.Put("dc11", "http://purl.org/dc/elements/1.1/", 10)

	got, err := mgr.GetWithBase("http://purl.org/dc/elements/1.1/")
	if err != nil {
		t.Fatalf("GetWithBase: %v", err)
	}
	if got.Prefix != "dc11" {
		t.Errorf("GetWithBase: prefix = %q, want %q", got.Prefix, "dc11")
	}
}

func TestManager_Prefixes(t *testing.T) {
	mgr := NewStore()
	mgr.Put("crm", "http://www.cidoc-crm.org/cidoc-crm/", 10)
	mgr.Put("rdf", "http://www.w3.org/1999/02/22-rdf-syntax-ns#", 10)

	prefixes := mgr.Prefixes()
	want := map[string]string{
		"crm": "http://www.cidoc-crm.org/cidoc-crm/",
		"rdf": "http://www.w3.org/1999/02/22-rdf-syntax-ns#",
	}
	if diff := cmp.Diff(want, prefixes); diff != "" {
		t.Errorf("Prefixes mismatch (-want +got):\n%s", diff)
	}
}
