// Package ontologyvendor adapts the ontology slice's Service into
// gitmaterializer.VendoredOntologyImporter. It is the single implementation
// of that adapter: pkg/app's production wiring and internal/testdb's
// integration-test wiring both call New here rather than each maintaining
// their own copy. This package is a leaf — it imports only
// pkg/service/gitmaterializer and pkg/weave/ontology, never pkg/app or
// internal/testdb, so both callers can depend on it without forming a cycle.
package ontologyvendor

import (
	"context"

	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// New adapts an ontology Service into the gitmaterializer.VendoredOntologyImporter
// interface, so a snapshot restore can hydrate vendored ontologies without
// gitmaterializer depending on the ontology slice directly.
func New(svc *weaveontology.Service) gitmaterializer.VendoredOntologyImporter {
	return importer{svc: svc}
}

// importer bridges gitmaterializer's restore seam to the ontology slice's
// primitives-only import entry point.
type importer struct{ svc *weaveontology.Service }

// ImportVendoredOntology adapts a gitmaterializer manifest into an
// ontology.VendoredOntologyImportRequest and imports it. BaseDir is the
// manifest's own root directory: writeVendoredOntology (vendor_snapshot.go)
// writes source files under "src/" relative to that root, and
// manifest.Sources.Files carries those same root-relative paths, so
// BaseDir+Files agree with no further adjustment.
func (a importer) ImportVendoredOntology(ctx context.Context, imp gitmaterializer.VendoredOntologyImport) error {
	root := imp.Manifest.Ontology
	var files []string
	if imp.Manifest.Sources != nil {
		files = imp.Manifest.Sources.Files
	}
	return a.svc.ImportVendoredVersion(ctx, weaveontology.VendoredOntologyImportRequest{
		OntologyID:    root.OntologyID,
		VersionID:     root.VersionID,
		VersionString: root.Version,
		Slug:          root.Slug,
		Title:         root.Title,
		Kind:          root.Kind,
		Namespace:     root.Namespace,
		Prefixes:      root.Prefixes,
		BaseDir:       imp.RootDir,
		Files:         files,
		Imports:       imp.Manifest.Imports,
	})
}
