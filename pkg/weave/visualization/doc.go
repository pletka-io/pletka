// Package visualization owns the per-entity diagram + RDF surface
// used by the detail-view's Diagram tab. It composes the existing
// pkg/weave/generators service to render Mermaid diagrams + Turtle /
// JSON-LD / SHACL flavours of RDF without duplicating snapshot logic. This
// package is a handler-only module: it composes pkg/weave/generators
// output for models and collections and owns no store.
//
// Routes (all gated by auth.RequireProjectRead):
//
//	GET /projects/{projectID}/models/{modelID}/diagram?mode=ontology|instance
//	GET /projects/{projectID}/models/{modelID}/rdf?format=turtle|jsonld|shacl
//	GET /projects/{projectID}/collections/{collectionID}/diagram?mode=ontology|instance
//	GET /projects/{projectID}/collections/{collectionID}/rdf?format=turtle|jsonld|shacl
//
// Replaces the older /api/v1/{models,collections}/{id}/{diagram,turtle}
// endpoints, which only knew about Mermaid + Turtle.
package visualization
