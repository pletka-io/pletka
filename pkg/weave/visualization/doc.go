// Package visualization owns the per-entity diagram + RDF surface
// used by the detail-view's Diagram tab. It composes the existing
// pkg/weave/generators service to render Mermaid diagrams + Turtle /
// JSON-LD / SHACL flavours of RDF without duplicating snapshot logic. This
// package is a handler-only module: it composes pkg/weave/generators
// output for models, collections and fields and owns no store.
//
// Routes authorize current consumer project access before selecting content:
//
//	GET /projects/{projectID}/gen/models/{modelID}/{format}?version=...
//	GET /projects/{projectID}/gen/collections/{collectionID}/{format}?version=...
//	GET /projects/{projectID}/gen/fields/{fieldID}/{format}?version=...
//
// Legacy /gen routes retain live owner discovery. Project-qualified routes need
// no live entity row and preserve the consumer context for inherited content.
package visualization
