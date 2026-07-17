package app

import (
	"context"

	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// NewOntologyVendorImporter adapts an ontology Service into the
// gitmaterializer.VendoredOntologyImporter interface, so a snapshot restore
// can hydrate vendored ontologies without gitmaterializer depending on the
// ontology slice directly.
func NewOntologyVendorImporter(svc *weaveontology.Service) gitmaterializer.VendoredOntologyImporter {
	return ontologyVendorImporter{svc: svc}
}

// ontologyVendorImporter bridges gitmaterializer's restore seam to the
// ontology slice's primitives-only import entry point.
type ontologyVendorImporter struct{ svc *weaveontology.Service }

// ImportVendoredOntology adapts a gitmaterializer manifest into an
// ontology.VendoredOntologyImportRequest and imports it. BaseDir is the
// manifest's own root directory: writeVendoredOntology (vendor_snapshot.go)
// writes source files under "src/" relative to that root, and
// manifest.Sources.Files carries those same root-relative paths, so
// BaseDir+Files agree with no further adjustment.
func (a ontologyVendorImporter) ImportVendoredOntology(ctx context.Context, imp gitmaterializer.VendoredOntologyImport) error {
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
