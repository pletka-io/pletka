package ontology_test

import (
	"context"
	"strings"
	"testing"

	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

func TestImportVendoredVersion_RejectsUnsafeFilePaths(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{name: "path traversal", file: "../escape.rdf"},
		{name: "nested path traversal", file: "sub/../../escape.rdf"},
		{name: "absolute path", file: "/etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No store/DB access is needed: the path guard must reject the
			// request before ImportVendoredVersion ever calls the store.
			svc := weaveontology.NewService(nil, nil, nil)

			req := weaveontology.VendoredOntologyImportRequest{
				OntologyID:    "unsafe-ontology",
				VersionID:     "unsafe-ontology-v1",
				VersionString: "1.0",
				Slug:          "unsafe-ontology",
				Namespace:     "https://example.org/unsafe/",
				Prefixes:      []string{"ux"},
				BaseDir:       t.TempDir(),
				Files:         []string{tt.file},
			}

			err := svc.ImportVendoredVersion(context.Background(), req)
			if err == nil {
				t.Fatal("ImportVendoredVersion() = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.file) {
				t.Fatalf("error %q does not name the unsafe file %q", err.Error(), tt.file)
			}
		})
	}
}
