// Package projectontologyversion implements the project-ontology-version
// vertical slice: data store, service, HTTP handler, form-schema builders,
// and route registration for the weave_project_ontology_versions junction
// table. This package is a full slice: it owns its store, service, handler,
// and routes.
//
// Mounted at /projects/{projectID}/project-ontology-versions by
// pkg/weave/router. The slice owns:
//
//   - Store + service for own row CRUD + primary-flip + usage gates.
//   - Composite read endpoint (own grouped + inherited from ancestors).
//   - Cascade-aware delete with field-usage 409 conflict shape.
//   - Auxiliary options endpoints for the create form's dependent
//     selects (versions for a base; extensions for a base version).
//   - List + form schema builders.
//
// Cross-slice reads still go through deps:
//   - deps.Weave.Projects() for project lookup + ancestor resolution.
//   - deps.Ontologies / deps.OntologyVersions for the ontology master
//     tables (satisfied by pkg/weave/ontology in the slice path; the
//     legacy GORM repository can also be wrapped via the adapter in
//     cmd/serve.go during the strangler-fig period).
package projectontologyversion
