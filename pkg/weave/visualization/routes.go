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
	Logger     *slog.Logger
	// LatestRelease backs the release-default policy applied in
	// loadAndGateProject so a public project's non-editor reader gets the
	// latest release instead of the hot draft, matching
	// auth.ResolveContentVersion. A missing reader fails reads that need
	// a default release. Both project-qualified routes and legacy owner-relative
	// routes use the same policy once the current project is authorized.
	LatestRelease weaveauth.LatestReleaseReader
}

func (h Host) Validate() error {
	if h.Generators == nil {
		return fmt.Errorf("visualization host missing required dependencies: Generators")
	}
	if h.Weave == nil {
		return fmt.Errorf("visualization host missing required dependencies: Weave")
	}
	return nil
}

// Mount registers the visualization slice routes on parent.
// Project-qualified routes preserve the consumer context and work without a live
// entity row. Legacy /gen routes retain owner discovery for existing links.
func Mount(parent chi.Router, h Host) {
	if err := h.Validate(); err != nil {
		panic(err)
	}
	handler := NewHandler(h.Weave, h.Generators, h.Logger, h.LatestRelease)

	withVersion := parent.With(weaveauth.WithProjectVersionContext)

	for _, prefix := range []string{"/projects/{projectID}", ""} {
		mountRoutes(withVersion, handler, prefix)
	}
}

func mountRoutes(router chi.Router, handler *Handler, prefix string) {
	router.Get(prefix+"/gen/models/{modelID}/diagram", handler.GetModelDiagram)
	router.Get(prefix+"/gen/models/{modelID}/turtle", handler.GetModelTurtle)
	router.Get(prefix+"/gen/models/{modelID}/jsonld", handler.GetModelJSONLD)
	router.Get(prefix+"/gen/models/{modelID}/shacl", handler.GetModelSHACL)
	router.Get(prefix+"/gen/models/{modelID}/exportgraph", handler.GetModelExportGraph)
	router.Get(prefix+"/gen/models/{modelID}/cytoscape", handler.GetModelCytoscape)
	router.Get(prefix+"/gen/models/{modelID}/sparql", handler.GetModelSPARQL)
	router.Get(prefix+"/gen/models/{modelID}/x3ml-a", handler.GetModelX3MLA)
	router.Get(prefix+"/gen/models/{modelID}/x3ml-b", handler.GetModelX3MLB)
	router.Get(prefix+"/gen/models/{modelID}/researchspace", handler.GetModelResearchSpace)
	router.Get(prefix+"/gen/models/{modelID}/arches", handler.GetModelArches)
	router.Get(prefix+"/gen/models/{modelID}/snapshot", handler.GetModelSnapshot)
	router.Get(prefix+"/gen/models/{modelID}/ascii-tree", handler.GetModelASCIITree)

	router.Get(prefix+"/gen/collections/{collectionID}/diagram", handler.GetCollectionDiagram)
	router.Get(prefix+"/gen/collections/{collectionID}/turtle", handler.GetCollectionTurtle)
	router.Get(prefix+"/gen/collections/{collectionID}/jsonld", handler.GetCollectionJSONLD)
	router.Get(prefix+"/gen/collections/{collectionID}/shacl", handler.GetCollectionSHACL)
	router.Get(prefix+"/gen/collections/{collectionID}/exportgraph", handler.GetCollectionExportGraph)
	router.Get(prefix+"/gen/collections/{collectionID}/cytoscape", handler.GetCollectionCytoscape)
	router.Get(prefix+"/gen/collections/{collectionID}/sparql", handler.GetCollectionSPARQL)
	router.Get(prefix+"/gen/collections/{collectionID}/x3ml-a", handler.GetCollectionX3MLA)
	router.Get(prefix+"/gen/collections/{collectionID}/x3ml-b", handler.GetCollectionX3MLB)

	router.Get(prefix+"/gen/fields/{fieldID}/diagram", handler.GetFieldDiagram)
	router.Get(prefix+"/gen/fields/{fieldID}/turtle", handler.GetFieldTurtle)
	router.Get(prefix+"/gen/fields/{fieldID}/jsonld", handler.GetFieldJSONLD)
	router.Get(prefix+"/gen/fields/{fieldID}/shacl", handler.GetFieldSHACL)
	router.Get(prefix+"/gen/fields/{fieldID}/exportgraph", handler.GetFieldExportGraph)
	router.Get(prefix+"/gen/fields/{fieldID}/cytoscape", handler.GetFieldCytoscape)
	router.Get(prefix+"/gen/fields/{fieldID}/sparql", handler.GetFieldSPARQL)
	router.Get(prefix+"/gen/fields/{fieldID}/x3ml-a", handler.GetFieldX3MLA)
	router.Get(prefix+"/gen/fields/{fieldID}/x3ml-b", handler.GetFieldX3MLB)
}
