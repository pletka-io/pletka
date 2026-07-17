// Package namespacebinding implements the Namespace-Binding vertical slice:
// data store, service, HTTP handler, form-schema builders, and route
// registration. Same shape as pkg/weave/category but smaller surface — a
// good second-iteration validation of the slice template. This package is a
// full slice: it owns its store, service, handler, and routes.
//
// A namespace binding is a (prefix → namespace URI) mapping used when
// resolving CURIE-style references (e.g. "crm:E21_Person"). Bindings come
// from three sources:
//
//   - "system": baked-in well-known prefixes (rdf, rdfs, owl, ...). Read-
//     only, project_id is NULL, visible in every project.
//   - "ontology" / "manifest" / etc.: imported from ontology pipelines.
//     Also read-only; project_id is set.
//   - "user": authored by project members through this slice. Mutable.
//
// The slice's mutating methods (Create / Update / Delete) only touch
// "user"-sourced rows. System / imported rows surface in List with
// _readonly=true so the UI hides their action buttons.
package namespacebinding
