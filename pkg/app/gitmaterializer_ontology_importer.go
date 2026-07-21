package app

import (
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer/ontologyvendor"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// NewOntologyVendorImporter adapts an ontology Service into the
// gitmaterializer.VendoredOntologyImporter interface, so a snapshot restore
// can hydrate vendored ontologies without gitmaterializer depending on the
// ontology slice directly. The adapter itself lives in the leaf package
// ontologyvendor so internal/testdb's integration-test wiring can share the
// same implementation without importing pkg/app.
func NewOntologyVendorImporter(svc *weaveontology.Service) gitmaterializer.VendoredOntologyImporter {
	return ontologyvendor.New(svc)
}
