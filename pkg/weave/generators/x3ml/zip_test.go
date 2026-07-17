package x3ml_test

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators/x3ml"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestBuildZip_MappingOnly(t *testing.T) {
	got, err := x3ml.BuildZip("mapping.a.x3ml", []byte("<x3ml/>"), nil)
	if err != nil {
		t.Fatalf("BuildZip: %v", err)
	}
	entries := readZip(t, got)
	want := map[string]string{"mapping.a.x3ml": "<x3ml/>"}
	if diff := cmp.Diff(want, entries); diff != "" {
		t.Fatalf("zip entries (-want +got):\n%s", diff)
	}
}

func TestBuildZip_WithBundle(t *testing.T) {
	bundle := []domain.OntologyBundleEntry{
		{OriginalFilename: "crm.ttl", RDFContent: "@prefix crm: <> ."},
		{OriginalFilename: "", RDFContent: "skipped — no filename"},
		{OriginalFilename: "empty.ttl", RDFContent: ""},
		{OriginalFilename: "crm.ttl", RDFContent: "duplicate — dedup"},
		{OriginalFilename: "skos.ttl", RDFContent: "@prefix skos: <> ."},
	}
	got, err := x3ml.BuildZip("mapping.b.x3ml", []byte("<x3ml/>"), bundle)
	if err != nil {
		t.Fatalf("BuildZip: %v", err)
	}
	entries := readZip(t, got)
	want := map[string]string{
		"mapping.b.x3ml": "<x3ml/>",
		"crm.ttl":        "@prefix crm: <> .",
		"skos.ttl":       "@prefix skos: <> .",
	}
	if diff := cmp.Diff(want, entries, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("zip entries (-want +got):\n%s", diff)
	}
}

func readZip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip open: %v", err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %q: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read %q: %v", f.Name, err)
		}
		out[f.Name] = string(b)
	}
	return out
}
