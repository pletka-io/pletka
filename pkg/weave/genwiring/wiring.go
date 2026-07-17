package genwiring

import (
	"github.com/pletka-io/pletka/pkg/weave/generators"
	weavecsv "github.com/pletka-io/pletka/pkg/weave/generators/csv"
	weaveexportgraph "github.com/pletka-io/pletka/pkg/weave/generators/exportgraph"
	weavemermaid "github.com/pletka-io/pletka/pkg/weave/generators/mermaid"
	weaverdf "github.com/pletka-io/pletka/pkg/weave/generators/rdf"
	weaveresearchspace "github.com/pletka-io/pletka/pkg/weave/generators/researchspace"
	weavesparql "github.com/pletka-io/pletka/pkg/weave/generators/sparql"
	weavex3ml "github.com/pletka-io/pletka/pkg/weave/generators/x3ml"
)

// CoreRenderers returns the generator renderers that belong to the
// public/core surface. Platform renderers (arches, shacl, cytoscape) are
// appended by host wiring in pletka-platform.
func CoreRenderers() []generators.Renderer {
	return []generators.Renderer{
		weaverdf.NewTurtleRenderer(),
		weaverdf.NewJSONLDRenderer(),
		weavemermaid.NewRenderer(),
		weaveexportgraph.NewRenderer(),
		weavesparql.NewRenderer(),
		weavex3ml.NewRendererA(),
		weavex3ml.NewRendererB(),
		weaveresearchspace.NewRenderer(),
	}
}

// CoreCLIRenderers returns the core renderer set plus command-only formats.
// CSV is exposed by `pletka weave generate` but not mounted in the server
// visualization tabs.
func CoreCLIRenderers() []generators.Renderer {
	return append([]generators.Renderer{weavecsv.NewRenderer()}, CoreRenderers()...)
}
