package visualization

import (
	"fmt"
	"log/slog"

	"github.com/go-chi/chi/v5"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

// Host is the explicit contract required by the visualization routes.
type Host struct {
	Generators *generators.Service
	Weave      domain.WeaveStore
	Bundles    ontologyBundleReader
	Logger     *slog.Logger
	// LatestRelease backs the release-default policy applied in
	// loadAndGateProject so a public project's non-editor reader gets the
	// latest release instead of the hot draft, matching
	// auth.ResolveContentVersion. Optional; nil disables the default
	// (readers see hot). Routes here have no {projectID} URL param, so
	// auth.ResolveContentVersion can't be installed as router middleware —
	// the same policy runs inline once the project is resolved.
	LatestRelease weaveauth.LatestReleaseReader
}

func (h Host) Validate() error {
	if h.Generators == nil {
		return fmt.Errorf("visualization host missing required dependencies: Generators")
	}
	if h.Weave == nil {
		return fmt.Errorf("visualization host missing required dependencies: Weave")
	}
	if h.Bundles == nil {
		return fmt.Errorf("visualization host missing required dependencies: Bundles")
	}
	return nil
}

// Mount registers the visualization slice routes on parent.
// Auth gating runs inside each handler because the URL has no {projectID} for
// middleware to read.
func Mount(parent chi.Router, h Host) {
	if err := h.Validate(); err != nil {
		panic(err)
	}
	handler := NewHandler(h.Weave, h.Generators, h.Bundles, h.Logger, h.LatestRelease)

	withVersion := parent.With(weaveauth.WithProjectVersionContext)

	withVersion.Get("/gen/models/{modelID}/diagram", handler.GetModelDiagram)
	withVersion.Get("/gen/models/{modelID}/turtle", handler.GetModelTurtle)
	withVersion.Get("/gen/models/{modelID}/jsonld", handler.GetModelJSONLD)
	withVersion.Get("/gen/models/{modelID}/shacl", handler.GetModelSHACL)
	withVersion.Get("/gen/models/{modelID}/exportgraph", handler.GetModelExportGraph)
	withVersion.Get("/gen/models/{modelID}/cytoscape", handler.GetModelCytoscape)
	withVersion.Get("/gen/models/{modelID}/sparql", handler.GetModelSPARQL)
	withVersion.Get("/gen/models/{modelID}/x3ml-a", handler.GetModelX3MLA)
	withVersion.Get("/gen/models/{modelID}/x3ml-b", handler.GetModelX3MLB)
	withVersion.Get("/gen/models/{modelID}/researchspace", handler.GetModelResearchSpace)
	withVersion.Get("/gen/models/{modelID}/arches", handler.GetModelArches)
	withVersion.Get("/gen/models/{modelID}/snapshot", handler.GetModelSnapshot)
	withVersion.Get("/gen/models/{modelID}/ascii-tree", handler.GetModelASCIITree)

	withVersion.Get("/gen/collections/{collectionID}/diagram", handler.GetCollectionDiagram)
	withVersion.Get("/gen/collections/{collectionID}/turtle", handler.GetCollectionTurtle)
	withVersion.Get("/gen/collections/{collectionID}/jsonld", handler.GetCollectionJSONLD)
	withVersion.Get("/gen/collections/{collectionID}/shacl", handler.GetCollectionSHACL)
	withVersion.Get("/gen/collections/{collectionID}/exportgraph", handler.GetCollectionExportGraph)
	withVersion.Get("/gen/collections/{collectionID}/cytoscape", handler.GetCollectionCytoscape)
	withVersion.Get("/gen/collections/{collectionID}/sparql", handler.GetCollectionSPARQL)
	withVersion.Get("/gen/collections/{collectionID}/x3ml-a", handler.GetCollectionX3MLA)
	withVersion.Get("/gen/collections/{collectionID}/x3ml-b", handler.GetCollectionX3MLB)

	withVersion.Get("/gen/fields/{fieldID}/diagram", handler.GetFieldDiagram)
	withVersion.Get("/gen/fields/{fieldID}/turtle", handler.GetFieldTurtle)
	withVersion.Get("/gen/fields/{fieldID}/jsonld", handler.GetFieldJSONLD)
	withVersion.Get("/gen/fields/{fieldID}/shacl", handler.GetFieldSHACL)
	withVersion.Get("/gen/fields/{fieldID}/exportgraph", handler.GetFieldExportGraph)
	withVersion.Get("/gen/fields/{fieldID}/cytoscape", handler.GetFieldCytoscape)
	withVersion.Get("/gen/fields/{fieldID}/sparql", handler.GetFieldSPARQL)
	withVersion.Get("/gen/fields/{fieldID}/x3ml-a", handler.GetFieldX3MLA)
	withVersion.Get("/gen/fields/{fieldID}/x3ml-b", handler.GetFieldX3MLB)
}
