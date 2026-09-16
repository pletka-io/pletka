package visualization

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/pletka-io/pletka/pkg/auth"
	pkgdomain "github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	weavex3ml "github.com/pletka-io/pletka/pkg/weave/generators/x3ml"
)

// Handler renders Mermaid diagrams + RDF (Turtle / JSON-LD / SHACL)
// for individual models and collections. Routes mount under /gen/ and
// each handler resolves the entity's project then runs the project-
// read auth check itself (the URL has no {projectID} so middleware
// gating is not possible).
type Handler struct {
	weave         pkgdomain.WeaveStore
	gens          *generators.Service
	bundles       ontologyBundleReader
	logger        *slog.Logger
	latestRelease auth.LatestReleaseReader
}

// ontologyBundleReader supplies a project's linked ontologies (with raw
// schema content) for the X3ML <target> blocks and ZIP download. The
// projectontologyversion slice's Store satisfies it.
type ontologyBundleReader interface {
	BundleForProject(ctx context.Context, projectID string) ([]pkgdomain.OntologyBundleEntry, error)
	BundleForVersions(ctx context.Context, versionIDs []string) ([]pkgdomain.OntologyBundleEntry, error)
}

type versionedModelLookup interface {
	GetByIDVersion(ctx context.Context, projectID, id, version string) (*pkgdomain.Model, error)
}

type versionedCollectionLookup interface {
	GetByIDVersion(ctx context.Context, projectID, id, version string) (*pkgdomain.Collection, error)
}

type versionedFieldLookup interface {
	GetByIDVersion(ctx context.Context, projectID, id, version string) (*pkgdomain.Field, error)
}

func NewHandler(weave pkgdomain.WeaveStore, gens *generators.Service, bundles ontologyBundleReader, logger *slog.Logger, latestRelease auth.LatestReleaseReader) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{weave: weave, gens: gens, bundles: bundles, logger: logger, latestRelease: latestRelease}
}

// DiagramResponse mirrors the legacy diagram JSON envelope so the
// frontend client.ts shape stays unchanged when URLs swap.
type DiagramResponse struct {
	Success bool   `json:"success"`
	Mermaid string `json:"mermaid,omitempty"`
	Name    string `json:"name,omitempty"`
	ID      string `json:"id,omitempty"`
	Count   int    `json:"count,omitempty"`
	Error   string `json:"error,omitempty"`
}

// GetModelDiagram handles GET /gen/models/{modelID}/diagram?mode=...
func (h *Handler) GetModelDiagram(w http.ResponseWriter, r *http.Request) {
	model, ctx, ok := h.loadModelAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, generators.FormatMermaid) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	opts, err := mermaidOptionsFromRequest(r)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	snap, err := h.gens.SnapshotForModel(ctx, model.ProjectID, model.ID, opts)
	if err != nil {
		h.logger.Error("model snapshot", "id", model.ID, "err", err)
		writeError(w, "failed to build model snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := h.gens.RenderSnapshot(ctx, generators.FormatMermaid, snap, &buf); err != nil {
		h.logger.Error("render model mermaid", "id", model.ID, "err", err)
		writeError(w, "failed to render diagram: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, DiagramResponse{
		Success: true,
		Mermaid: buf.String(),
		Name:    model.UIName.Get("en", model.SystemName),
		ID:      model.ID,
		Count:   len(snap.Fields),
	})
}

// GetCollectionDiagram handles GET /gen/collections/{collectionID}/diagram?mode=...
func (h *Handler) GetCollectionDiagram(w http.ResponseWriter, r *http.Request) {
	coll, ctx, ok := h.loadCollectionAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, generators.FormatMermaid) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	opts, err := mermaidOptionsFromRequest(r)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	snap, err := h.gens.SnapshotForCollection(ctx, coll.ProjectID, coll.ID, opts)
	if err != nil {
		h.logger.Error("collection snapshot", "id", coll.ID, "err", err)
		writeError(w, "failed to build collection snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := h.gens.RenderSnapshot(ctx, generators.FormatMermaid, snap, &buf); err != nil {
		h.logger.Error("render collection mermaid", "id", coll.ID, "err", err)
		writeError(w, "failed to render diagram: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, DiagramResponse{
		Success: true,
		Mermaid: buf.String(),
		Name:    coll.UIName.Get("en", coll.SystemName),
		ID:      coll.ID,
		Count:   len(snap.Fields),
	})
}

// GetModelTurtle handles GET /gen/models/{modelID}/turtle.
func (h *Handler) GetModelTurtle(w http.ResponseWriter, r *http.Request) {
	h.modelRDF(w, r, generators.FormatTurtle, "text/turtle; charset=utf-8")
}

// GetModelJSONLD handles GET /gen/models/{modelID}/jsonld.
func (h *Handler) GetModelJSONLD(w http.ResponseWriter, r *http.Request) {
	h.modelRDF(w, r, generators.FormatJSONLD, "application/ld+json; charset=utf-8")
}

// GetModelSHACL handles GET /gen/models/{modelID}/shacl.
// SHACL shapes are serialised as Turtle, so the wire format is the
// same as /turtle but the route is distinct so the contract is
// explicit.
func (h *Handler) GetModelSHACL(w http.ResponseWriter, r *http.Request) {
	h.modelRDF(w, r, generators.FormatSHACL, "text/turtle; charset=utf-8")
}

// GetCollectionTurtle handles GET /gen/collections/{collectionID}/turtle.
func (h *Handler) GetCollectionTurtle(w http.ResponseWriter, r *http.Request) {
	h.collectionRDF(w, r, generators.FormatTurtle, "text/turtle; charset=utf-8")
}

// GetCollectionJSONLD handles GET /gen/collections/{collectionID}/jsonld.
func (h *Handler) GetCollectionJSONLD(w http.ResponseWriter, r *http.Request) {
	h.collectionRDF(w, r, generators.FormatJSONLD, "application/ld+json; charset=utf-8")
}

// GetCollectionSHACL handles GET /gen/collections/{collectionID}/shacl.
func (h *Handler) GetCollectionSHACL(w http.ResponseWriter, r *http.Request) {
	h.collectionRDF(w, r, generators.FormatSHACL, "text/turtle; charset=utf-8")
}

// GetModelExportGraph handles GET /gen/models/{modelID}/exportgraph.
// Per-format capability gating happens inside modelRDF; the export-
// graph format requires auth.DerivativeExportGraphRead. Defense-in-
// depth: derivativesFor in detailview/handler.go omits the URL from
// the schema for callers without the capability, this server-side
// gate catches anyone who crafts the URL by hand.
func (h *Handler) GetModelExportGraph(w http.ResponseWriter, r *http.Request) {
	h.modelRDF(w, r, generators.FormatExportGraph, "application/json; charset=utf-8")
}

// GetCollectionExportGraph handles GET /gen/collections/{collectionID}/exportgraph.
// Per-format capability gating happens inside collectionRDF.
func (h *Handler) GetCollectionExportGraph(w http.ResponseWriter, r *http.Request) {
	h.collectionRDF(w, r, generators.FormatExportGraph, "application/json; charset=utf-8")
}

// GetFieldDiagram handles GET /gen/fields/{fieldID}/diagram?mode=...
// A field is a single ontology path; the mode toggle (ontology vs
// instance) that matters for models/collections that aggregate paths
// is parsed but exercises nothing meaningful for fields. Callers
// passing the query param still work.
func (h *Handler) GetFieldDiagram(w http.ResponseWriter, r *http.Request) {
	field, ctx, ok := h.loadFieldAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, generators.FormatMermaid) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	opts, err := mermaidOptionsFromRequest(r)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	snap, err := h.gens.SnapshotForField(ctx, field.ProjectID, field.ID, opts)
	if err != nil {
		h.logger.Error("field snapshot", "id", field.ID, "err", err)
		writeError(w, "failed to build field snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := h.gens.RenderSnapshot(ctx, generators.FormatMermaid, snap, &buf); err != nil {
		h.logger.Error("render field mermaid", "id", field.ID, "err", err)
		writeError(w, "failed to render diagram: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, DiagramResponse{
		Success: true,
		Mermaid: buf.String(),
		Name:    field.UIName.Get("en", field.SystemName),
		ID:      field.ID,
		Count:   len(snap.Fields),
	})
}

// GetFieldTurtle handles GET /gen/fields/{fieldID}/turtle.
func (h *Handler) GetFieldTurtle(w http.ResponseWriter, r *http.Request) {
	h.fieldRDF(w, r, generators.FormatTurtle, "text/turtle; charset=utf-8")
}

// GetFieldJSONLD handles GET /gen/fields/{fieldID}/jsonld.
func (h *Handler) GetFieldJSONLD(w http.ResponseWriter, r *http.Request) {
	h.fieldRDF(w, r, generators.FormatJSONLD, "application/ld+json; charset=utf-8")
}

// GetFieldSHACL handles GET /gen/fields/{fieldID}/shacl.
func (h *Handler) GetFieldSHACL(w http.ResponseWriter, r *http.Request) {
	h.fieldRDF(w, r, generators.FormatSHACL, "text/turtle; charset=utf-8")
}

// GetFieldExportGraph handles GET /gen/fields/{fieldID}/exportgraph.
// Per-format capability gating happens inside fieldRDF.
func (h *Handler) GetFieldExportGraph(w http.ResponseWriter, r *http.Request) {
	h.fieldRDF(w, r, generators.FormatExportGraph, "application/json; charset=utf-8")
}

// GetModelCytoscape handles GET /gen/models/{modelID}/cytoscape?mode=...
// Cytoscape is the parallel graph format alongside Mermaid; the
// frontend renderer for it is not yet shipped — the endpoint is live
// so Ekjs and downstream consumers can call it directly. Same auth
// gate as Mermaid (any project reader).
func (h *Handler) GetModelCytoscape(w http.ResponseWriter, r *http.Request) {
	h.modelGraph(w, r, generators.FormatCytoscape)
}

// GetCollectionCytoscape handles GET /gen/collections/{collectionID}/cytoscape?mode=...
func (h *Handler) GetCollectionCytoscape(w http.ResponseWriter, r *http.Request) {
	h.collectionGraph(w, r, generators.FormatCytoscape)
}

// GetFieldCytoscape handles GET /gen/fields/{fieldID}/cytoscape?mode=...
func (h *Handler) GetFieldCytoscape(w http.ResponseWriter, r *http.Request) {
	h.fieldGraph(w, r, generators.FormatCytoscape)
}

// GetModelSPARQL handles GET /gen/models/{modelID}/sparql.
// Honours ?count=1 (COUNT projection) and ?limit=N (default 100).
func (h *Handler) GetModelSPARQL(w http.ResponseWriter, r *http.Request) {
	h.modelSPARQL(w, r)
}

// GetCollectionSPARQL handles GET /gen/collections/{collectionID}/sparql.
func (h *Handler) GetCollectionSPARQL(w http.ResponseWriter, r *http.Request) {
	h.collectionSPARQL(w, r)
}

// GetFieldSPARQL handles GET /gen/fields/{fieldID}/sparql.
func (h *Handler) GetFieldSPARQL(w http.ResponseWriter, r *http.Request) {
	h.fieldSPARQL(w, r)
}

func (h *Handler) modelSPARQL(w http.ResponseWriter, r *http.Request) {
	model, ctx, ok := h.loadModelAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, generators.FormatSPARQL) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	opts := sparqlOptionsFromRequest(r)
	snap, err := h.gens.SnapshotForModel(ctx, model.ProjectID, model.ID, opts)
	if err != nil {
		h.logger.Error("model snapshot", "id", model.ID, "err", err)
		writeError(w, "failed to build model snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeSPARQL(ctx, w, snap)
}

func (h *Handler) collectionSPARQL(w http.ResponseWriter, r *http.Request) {
	coll, ctx, ok := h.loadCollectionAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, generators.FormatSPARQL) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	opts := sparqlOptionsFromRequest(r)
	snap, err := h.gens.SnapshotForCollection(ctx, coll.ProjectID, coll.ID, opts)
	if err != nil {
		h.logger.Error("collection snapshot", "id", coll.ID, "err", err)
		writeError(w, "failed to build collection snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeSPARQL(ctx, w, snap)
}

func (h *Handler) fieldSPARQL(w http.ResponseWriter, r *http.Request) {
	field, ctx, ok := h.loadFieldAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, generators.FormatSPARQL) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	opts := sparqlOptionsFromRequest(r)
	snap, err := h.gens.SnapshotForField(ctx, field.ProjectID, field.ID, opts)
	if err != nil {
		h.logger.Error("field snapshot", "id", field.ID, "err", err)
		writeError(w, "failed to build field snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeSPARQL(ctx, w, snap)
}

// X3ML form A handlers — single mapping, full path links.

func (h *Handler) GetModelX3MLA(w http.ResponseWriter, r *http.Request) {
	h.modelX3ML(w, r, generators.FormatX3ML)
}

func (h *Handler) GetCollectionX3MLA(w http.ResponseWriter, r *http.Request) {
	h.collectionX3ML(w, r, generators.FormatX3ML)
}

func (h *Handler) GetFieldX3MLA(w http.ResponseWriter, r *http.Request) {
	h.fieldX3ML(w, r, generators.FormatX3ML)
}

// X3ML form B handlers — domain split into spine + tail mappings.

func (h *Handler) GetModelX3MLB(w http.ResponseWriter, r *http.Request) {
	h.modelX3ML(w, r, generators.FormatX3MLB)
}

func (h *Handler) GetCollectionX3MLB(w http.ResponseWriter, r *http.Request) {
	h.collectionX3ML(w, r, generators.FormatX3MLB)
}

func (h *Handler) GetFieldX3MLB(w http.ResponseWriter, r *http.Request) {
	h.fieldX3ML(w, r, generators.FormatX3MLB)
}

func (h *Handler) modelX3ML(w http.ResponseWriter, r *http.Request, format generators.Format) {
	model, ctx, ok := h.loadModelAndGate(w, r)
	if !ok {
		return
	}
	h.serveX3ML(ctx, w, r, format, model.ProjectID, func(opts generators.Options) (*generators.Snapshot, error) {
		return h.gens.SnapshotForModel(ctx, model.ProjectID, model.ID, opts)
	})
}

func (h *Handler) collectionX3ML(w http.ResponseWriter, r *http.Request, format generators.Format) {
	coll, ctx, ok := h.loadCollectionAndGate(w, r)
	if !ok {
		return
	}
	h.serveX3ML(ctx, w, r, format, coll.ProjectID, func(opts generators.Options) (*generators.Snapshot, error) {
		return h.gens.SnapshotForCollection(ctx, coll.ProjectID, coll.ID, opts)
	})
}

func (h *Handler) fieldX3ML(w http.ResponseWriter, r *http.Request, format generators.Format) {
	field, ctx, ok := h.loadFieldAndGate(w, r)
	if !ok {
		return
	}
	h.serveX3ML(ctx, w, r, format, field.ProjectID, func(opts generators.Options) (*generators.Snapshot, error) {
		return h.gens.SnapshotForField(ctx, field.ProjectID, field.ID, opts)
	})
}

// serveX3ML loads the project's linked ontologies (own + inherited), builds
// the snapshot with one X3ML <target> per ontology, renders the mapping,
// then either streams the .x3ml document or — when ?bundle=zip — a ZIP
// carrying the mapping plus each ontology's raw schema file.
func (h *Handler) serveX3ML(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	format generators.Format,
	projectID string,
	build func(generators.Options) (*generators.Snapshot, error),
) {
	if !h.canDerivativeFormat(ctx, format) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	resolved, err := h.weave.Projects().ResolvedOntologyVersions(ctx, projectID, pkgdomain.ResolvedOntologyVersionOpts{})
	if err != nil {
		h.logger.Error("x3ml resolve ontology versions", "project", projectID, "err", err)
		writeError(w, "failed to resolve project ontologies: "+err.Error(), http.StatusInternalServerError)
		return
	}
	versionIDs := make([]string, 0, len(resolved))
	for _, r := range resolved {
		versionIDs = append(versionIDs, r.Link.OntologyVersionID)
	}
	bundle, err := h.bundles.BundleForVersions(ctx, versionIDs)
	if err != nil {
		h.logger.Error("x3ml ontology bundle", "project", projectID, "err", err)
		writeError(w, "failed to load project ontologies: "+err.Error(), http.StatusInternalServerError)
		return
	}
	snap, err := build(generators.Options{X3MLTargets: x3mlTargets(bundle)})
	if err != nil {
		h.logger.Error("x3ml snapshot", "project", projectID, "err", err)
		writeError(w, "failed to build snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := h.gens.RenderSnapshot(ctx, format, snap, &buf); err != nil {
		h.logger.Error("render x3ml", "format", format, "err", err)
		writeError(w, "failed to render x3ml: "+err.Error(), http.StatusInternalServerError)
		return
	}

	base := x3mlBaseName(snap) + x3mlExtension(format)
	if r.URL.Query().Get("bundle") == "zip" {
		h.writeX3MLZip(w, base, buf.Bytes(), bundle)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// writeX3MLZip streams a ZIP containing the rendered mapping plus one
// raw schema file per linked ontology that carries imported content.
// The zip assembly itself lives in pkg/weave/generators/x3ml so the
// integrations hub can reuse it as an artifact provider without going
// through the HTTP handler.
func (h *Handler) writeX3MLZip(w http.ResponseWriter, x3mlName string, x3ml []byte, bundle []pkgdomain.OntologyBundleEntry) {
	data, err := weavex3ml.BuildZip(x3mlName, x3ml, bundle)
	if err != nil {
		h.logger.Error("x3ml zip", "entry", x3mlName, "err", err)
		writeError(w, "failed to build zip: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", x3mlName+".zip"))
	_, _ = w.Write(data)
}

// x3mlTargets maps the project's linked ontologies to renderer targets.
func x3mlTargets(bundle []pkgdomain.OntologyBundleEntry) []generators.X3MLTarget {
	out := make([]generators.X3MLTarget, 0, len(bundle))
	for _, o := range bundle {
		out = append(out, generators.X3MLTarget{
			Prefix:     o.Prefix,
			Namespace:  o.Namespace,
			Label:      o.Name,
			Version:    o.VersionString,
			SchemaFile: o.OriginalFilename,
		})
	}
	return out
}

// x3mlBaseName is the entity-derived file stem for the download.
func x3mlBaseName(snap *generators.Snapshot) string {
	switch snap.RootKind {
	case generators.EntityModel:
		if snap.Model != nil {
			return firstNonEmpty(snap.Model.SemanticID, snap.Model.SystemName, snap.Model.ID)
		}
	case generators.EntityCollection:
		if snap.Collection != nil {
			return firstNonEmpty(snap.Collection.SemanticID, snap.Collection.SystemName, snap.Collection.ID)
		}
	case generators.EntityField:
		if snap.Field != nil {
			return firstNonEmpty(snap.Field.SemanticID, snap.Field.SystemName, snap.Field.ID)
		}
	}
	return "mapping"
}

func x3mlExtension(format generators.Format) string {
	if format == generators.FormatX3MLB {
		return ".b.x3ml"
	}
	return ".a.x3ml"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// GetModelArches handles GET /gen/models/{modelID}/arches.
// Emits the Arches 7.6 Resource Graph JSON document for the model.
// Super-admin only — the Arches integration surface is under active
// development and gated until the contract stabilises.
func (h *Handler) GetModelArches(w http.ResponseWriter, r *http.Request) {
	model, ctx, ok := h.loadModelAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, generators.FormatArches) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	snap, err := h.gens.SnapshotForModel(ctx, model.ProjectID, model.ID, generators.Options{})
	if err != nil {
		h.logger.Error("model snapshot", "id", model.ID, "err", err)
		writeError(w, "failed to build model snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := h.gens.RenderSnapshot(ctx, generators.FormatArches, snap, &buf); err != nil {
		h.logger.Error("render arches", "id", model.ID, "err", err)
		writeError(w, "failed to render arches: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprint(w, buf.String())
}

// GetModelSnapshot handles GET /gen/models/{modelID}/snapshot.
// Emits the renderer-neutral Snapshot JSON — the exact input every
// generator consumes, including the views.Tree with each path element's
// generated path_node / path_node_id. Super-admin only; a verification
// surface for the node-identity work.
func (h *Handler) GetModelSnapshot(w http.ResponseWriter, r *http.Request) {
	model, ctx, ok := h.loadModelAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, generators.FormatSnapshot) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	snap, err := h.gens.SnapshotForModel(ctx, model.ProjectID, model.ID, generators.Options{})
	if err != nil {
		h.logger.Error("model snapshot", "id", model.ID, "err", err)
		writeError(w, "failed to build model snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(snap); err != nil {
		h.logger.Error("encode model snapshot", "id", model.ID, "err", err)
	}
}

// GetModelASCIITree handles GET /gen/models/{modelID}/ascii-tree.
// Emits the renderer-neutral Snapshot as a human-readable ASCII tree
// keyed by PathNodeID/InstanceID, so a curator can review how every
// semantic intermediate is named and how every leaf field is grouped.
// Super-admin only — same gate as /snapshot and /arches.
func (h *Handler) GetModelASCIITree(w http.ResponseWriter, r *http.Request) {
	model, ctx, ok := h.loadModelAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, generators.FormatASCIITree) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	snap, err := h.gens.SnapshotForModel(ctx, model.ProjectID, model.ID, generators.Options{})
	if err != nil {
		h.logger.Error("model snapshot", "id", model.ID, "err", err)
		writeError(w, "failed to build model snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := generators.WriteSnapshotASCII(w, snap); err != nil {
		h.logger.Error("write ascii tree", "id", model.ID, "err", err)
	}
}

// GetModelResearchSpace handles GET /gen/models/{modelID}/researchspace.
// Emits the YAML config ResearchSpace consumes for field discovery /
// faceting, with one embedded SPARQL SELECT per resolved field.
func (h *Handler) GetModelResearchSpace(w http.ResponseWriter, r *http.Request) {
	model, ctx, ok := h.loadModelAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, generators.FormatResearchSpace) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	snap, err := h.gens.SnapshotForModel(ctx, model.ProjectID, model.ID, generators.Options{})
	if err != nil {
		h.logger.Error("model snapshot", "id", model.ID, "err", err)
		writeError(w, "failed to build model snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := h.gens.RenderSnapshot(ctx, generators.FormatResearchSpace, snap, &buf); err != nil {
		h.logger.Error("render researchspace", "id", model.ID, "err", err)
		writeError(w, "failed to render researchspace: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	fmt.Fprint(w, buf.String())
}

func (h *Handler) writeSPARQL(ctx context.Context, w http.ResponseWriter, snap *generators.Snapshot) {
	var buf bytes.Buffer
	if err := h.gens.RenderSnapshot(ctx, generators.FormatSPARQL, snap, &buf); err != nil {
		h.logger.Error("render sparql", "err", err)
		writeError(w, "failed to render sparql: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/sparql-query; charset=utf-8")
	fmt.Fprint(w, buf.String())
}

// modelGraph renders a graph derivative (e.g. Cytoscape) for a model.
// Honours the `mode` query (`ontology` | `instance`) the same way the
// mermaid endpoint does. JSON content type; routes through the derivative
// policy (cytoscape is open, but a future graph format is gated by default).
func (h *Handler) modelGraph(w http.ResponseWriter, r *http.Request, format generators.Format) {
	model, ctx, ok := h.loadModelAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, format) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	opts, err := mermaidOptionsFromRequest(r)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	snap, err := h.gens.SnapshotForModel(ctx, model.ProjectID, model.ID, opts)
	if err != nil {
		h.logger.Error("model snapshot", "id", model.ID, "err", err)
		writeError(w, "failed to build model snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := h.gens.RenderSnapshot(ctx, format, snap, &buf); err != nil {
		h.logger.Error("render model graph", "id", model.ID, "format", format, "err", err)
		writeError(w, fmt.Sprintf("failed to render %s: %s", format, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprint(w, buf.String())
}

func (h *Handler) collectionGraph(w http.ResponseWriter, r *http.Request, format generators.Format) {
	coll, ctx, ok := h.loadCollectionAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, format) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	opts, err := mermaidOptionsFromRequest(r)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	snap, err := h.gens.SnapshotForCollection(ctx, coll.ProjectID, coll.ID, opts)
	if err != nil {
		h.logger.Error("collection snapshot", "id", coll.ID, "err", err)
		writeError(w, "failed to build collection snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := h.gens.RenderSnapshot(ctx, format, snap, &buf); err != nil {
		h.logger.Error("render collection graph", "id", coll.ID, "format", format, "err", err)
		writeError(w, fmt.Sprintf("failed to render %s: %s", format, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprint(w, buf.String())
}

func (h *Handler) fieldGraph(w http.ResponseWriter, r *http.Request, format generators.Format) {
	field, ctx, ok := h.loadFieldAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, format) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	opts, err := mermaidOptionsFromRequest(r)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	snap, err := h.gens.SnapshotForField(ctx, field.ProjectID, field.ID, opts)
	if err != nil {
		h.logger.Error("field snapshot", "id", field.ID, "err", err)
		writeError(w, "failed to build field snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := h.gens.RenderSnapshot(ctx, format, snap, &buf); err != nil {
		h.logger.Error("render field graph", "id", field.ID, "format", format, "err", err)
		writeError(w, fmt.Sprintf("failed to render %s: %s", format, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprint(w, buf.String())
}

// canDerivativeFormat returns true when the caller has the capability
// the given derivative format requires. Most formats are open to any
// project reader; some (e.g. exportgraph) are gated on a fine-grained
// capability the server uses to decide both schema-URL emission (in
// detailview/handler.go's derivativesFor) AND raw-URL access here.
//
// The project resource is read from context — set by
// loadModelAndGate / loadCollectionAndGate / loadFieldAndGate via
// auth.WithProject. Returns 404 (not 403) at the call site so we
// don't leak the existence of the surface to callers without the
// capability.
// Enforcement is fail closed: the policy lives in auth.CanDerivative, which
// gates every format not explicitly opened. Each generator handler calls this
// after loading the entity (so the project resource is in context), and the
// detailview URL emit consults the same auth.CanDerivative — a format can't be
// gated in one exit point and open in the other. Returns 404 (not 403) at the
// call site so we don't leak the surface's existence to callers without access.
func (h *Handler) canDerivativeFormat(ctx context.Context, format generators.Format) bool {
	return auth.FromContext(ctx).CanDerivative(string(format), auth.ProjectResourceFromContext(ctx))
}

func (h *Handler) fieldRDF(w http.ResponseWriter, r *http.Request, format generators.Format, contentType string) {
	field, ctx, ok := h.loadFieldAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, format) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	snap, err := h.gens.SnapshotForField(ctx, field.ProjectID, field.ID, generators.Options{})
	if err != nil {
		h.logger.Error("field snapshot", "id", field.ID, "err", err)
		writeError(w, "failed to build field snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := h.gens.RenderSnapshot(ctx, format, snap, &buf); err != nil {
		h.logger.Error("render field rdf", "id", field.ID, "format", format, "err", err)
		writeError(w, fmt.Sprintf("failed to render %s: %s", format, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	fmt.Fprint(w, buf.String())
}

func (h *Handler) modelRDF(w http.ResponseWriter, r *http.Request, format generators.Format, contentType string) {
	model, ctx, ok := h.loadModelAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, format) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	snap, err := h.gens.SnapshotForModel(ctx, model.ProjectID, model.ID, generators.Options{})
	if err != nil {
		h.logger.Error("model snapshot", "id", model.ID, "err", err)
		writeError(w, "failed to build model snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := h.gens.RenderSnapshot(ctx, format, snap, &buf); err != nil {
		h.logger.Error("render model rdf", "id", model.ID, "format", format, "err", err)
		writeError(w, fmt.Sprintf("failed to render %s: %s", format, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	fmt.Fprint(w, buf.String())
}

func (h *Handler) collectionRDF(w http.ResponseWriter, r *http.Request, format generators.Format, contentType string) {
	coll, ctx, ok := h.loadCollectionAndGate(w, r)
	if !ok {
		return
	}
	if !h.canDerivativeFormat(ctx, format) {
		writeError(w, "not found", http.StatusNotFound)
		return
	}
	snap, err := h.gens.SnapshotForCollection(ctx, coll.ProjectID, coll.ID, generators.Options{})
	if err != nil {
		h.logger.Error("collection snapshot", "id", coll.ID, "err", err)
		writeError(w, "failed to build collection snapshot: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := h.gens.RenderSnapshot(ctx, format, snap, &buf); err != nil {
		h.logger.Error("render collection rdf", "id", coll.ID, "format", format, "err", err)
		writeError(w, fmt.Sprintf("failed to render %s: %s", format, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	fmt.Fprint(w, buf.String())
}

// loadModelAndGate fetches the model, then resolves + gates the project.
// 404s on any failure to avoid leaking presence of restricted projects.
//
// Returns a context with the project attached via auth.WithProject so
// downstream services that read project visibility from context (e.g.
// the namespacebinding service used during snapshot building) see the
// same Resource the gate check just authorised against. Without this,
// anonymous viewers on a public project passed the gate but tripped
// over an empty context-Resource deeper in the call chain, surfacing
// "forbidden: requires project.read".
func (h *Handler) loadModelAndGate(w http.ResponseWriter, r *http.Request) (*pkgdomain.Model, context.Context, bool) {
	ctx := r.Context()
	modelID := chi.URLParam(r, "modelID")
	if modelID == "" {
		writeError(w, "model ID is required", http.StatusBadRequest)
		return nil, ctx, false
	}
	model, err := h.weave.Models().GetByID(ctx, modelID)
	if err != nil || model == nil {
		writeError(w, "model not found", http.StatusNotFound)
		return nil, ctx, false
	}
	project, ok := h.loadAndGateProject(ctx, model.ProjectID)
	if !ok {
		writeError(w, "model not found", http.StatusNotFound)
		return nil, ctx, false
	}
	ctx = auth.WithProject(ctx, project)
	if version := h.effectiveVersion(ctx, project); version != "" {
		if vr, ok := h.weave.Models().(versionedModelLookup); ok {
			versioned, err := vr.GetByIDVersion(ctx, model.ProjectID, modelID, version)
			if err != nil {
				h.logger.Error("load archived model", "id", modelID, "project_id", model.ProjectID, "version", version, "err", err)
				writeError(w, "model not found", http.StatusNotFound)
				return nil, ctx, false
			}
			if versioned == nil {
				writeError(w, "model not found", http.StatusNotFound)
				return nil, ctx, false
			}
			model = versioned
		}
	}
	return model, ctx, true
}

func (h *Handler) loadCollectionAndGate(w http.ResponseWriter, r *http.Request) (*pkgdomain.Collection, context.Context, bool) {
	ctx := r.Context()
	collectionID := chi.URLParam(r, "collectionID")
	if collectionID == "" {
		writeError(w, "collection ID is required", http.StatusBadRequest)
		return nil, ctx, false
	}
	coll, err := h.weave.Collections().GetByID(ctx, collectionID)
	if err != nil || coll == nil {
		writeError(w, "collection not found", http.StatusNotFound)
		return nil, ctx, false
	}
	project, ok := h.loadAndGateProject(ctx, coll.ProjectID)
	if !ok {
		writeError(w, "collection not found", http.StatusNotFound)
		return nil, ctx, false
	}
	ctx = auth.WithProject(ctx, project)
	if version := h.effectiveVersion(ctx, project); version != "" {
		if vr, ok := h.weave.Collections().(versionedCollectionLookup); ok {
			versioned, err := vr.GetByIDVersion(ctx, coll.ProjectID, collectionID, version)
			if err != nil {
				h.logger.Error("load archived collection", "id", collectionID, "project_id", coll.ProjectID, "version", version, "err", err)
				writeError(w, "collection not found", http.StatusNotFound)
				return nil, ctx, false
			}
			if versioned == nil {
				writeError(w, "collection not found", http.StatusNotFound)
				return nil, ctx, false
			}
			coll = versioned
		}
	}
	return coll, ctx, true
}

func (h *Handler) loadFieldAndGate(w http.ResponseWriter, r *http.Request) (*pkgdomain.Field, context.Context, bool) {
	ctx := r.Context()
	fieldID := chi.URLParam(r, "fieldID")
	if fieldID == "" {
		writeError(w, "field ID is required", http.StatusBadRequest)
		return nil, ctx, false
	}
	field, err := h.weave.WeaveFields().GetByID(ctx, fieldID)
	if err != nil || field == nil {
		writeError(w, "field not found", http.StatusNotFound)
		return nil, ctx, false
	}
	project, ok := h.loadAndGateProject(ctx, field.ProjectID)
	if !ok {
		writeError(w, "field not found", http.StatusNotFound)
		return nil, ctx, false
	}
	ctx = auth.WithProject(ctx, project)
	if version := h.effectiveVersion(ctx, project); version != "" {
		if vr, ok := h.weave.WeaveFields().(versionedFieldLookup); ok {
			versioned, err := vr.GetByIDVersion(ctx, field.ProjectID, fieldID, version)
			if err != nil {
				h.logger.Error("load archived field", "id", fieldID, "project_id", field.ProjectID, "version", version, "err", err)
				writeError(w, "field not found", http.StatusNotFound)
				return nil, ctx, false
			}
			if versioned == nil {
				writeError(w, "field not found", http.StatusNotFound)
				return nil, ctx, false
			}
			field = versioned
		}
	}
	return field, ctx, true
}

// loadAndGateProject loads projectID and verifies the caller has
// project.read on it. Returns the loaded project on success so callers
// can attach it to the request context for downstream services.
func (h *Handler) loadAndGateProject(ctx context.Context, projectID string) (*pkgdomain.Project, bool) {
	project, err := h.weave.Projects().GetByID(ctx, projectID)
	if err != nil || project == nil {
		return nil, false
	}
	if !auth.FromContext(ctx).Can(auth.ProjectRead, auth.ProjectResource(project), nil) {
		return nil, false
	}
	return project, true
}

// effectiveVersion mirrors auth.ResolveContentVersion's policy for routes
// where the project can only be resolved from inside the handler (these
// /gen/ routes have no {projectID} URL param, so the middleware form can't
// be installed on the router — see the Host.LatestRelease doc comment).
// Returns "" for hot, an explicit ?version= value verbatim, or the latest
// release for a public project's non-editor reader.
func (h *Handler) effectiveVersion(ctx context.Context, project *pkgdomain.Project) string {
	if explicit := auth.ProjectVersionFromContext(ctx); explicit != "" {
		return explicit
	}
	if h.latestRelease == nil || project == nil || project.Visibility != "public" {
		return ""
	}
	snap := auth.FromContext(ctx)
	if snap.Can(auth.ProjectEdit, auth.ProjectResource(project), nil) {
		return "" // editor: hot
	}
	latest, err := h.latestRelease.LatestReleaseVersion(ctx, project.ID)
	if err != nil {
		h.logger.ErrorContext(ctx, "resolve content version: latest release lookup failed; serving hot", "project", project.ID, "err", err)
		return ""
	}
	return auth.ResolveEffectiveVersion(snap, project, "", latest)
}

// sparqlOptionsFromRequest reads ?count and ?limit from the URL.
// Default LIMIT is 100 (matches the legacy Python default); pass
// ?limit=0 to suppress the LIMIT clause; ?count=1 swaps the SELECT
// for a COUNT(?value) projection and skips the LIMIT.
func sparqlOptionsFromRequest(r *http.Request) generators.Options {
	opts := generators.Options{SPARQLLimit: 100}
	switch strings.ToLower(r.URL.Query().Get("count")) {
	case "1", "true", "yes":
		opts.SPARQLCount = true
		opts.SPARQLLimit = 0
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n >= 0 {
			opts.SPARQLLimit = n
		}
	}
	return opts
}

func mermaidOptionsFromRequest(r *http.Request) (generators.Options, error) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = string(generators.MermaidModeOntology)
	}
	parsed, err := parseMermaidMode(mode)
	if err != nil {
		return generators.Options{}, err
	}
	return generators.Options{MermaidMode: parsed}, nil
}

func parseMermaidMode(value string) (generators.MermaidMode, error) {
	switch value {
	case string(generators.MermaidModeOntology):
		return generators.MermaidModeOntology, nil
	case string(generators.MermaidModeInstance):
		return generators.MermaidModeInstance, nil
	default:
		return "", fmt.Errorf("unsupported mode %q (want ontology or instance)", value)
	}
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(DiagramResponse{Success: false, Error: msg})
}
