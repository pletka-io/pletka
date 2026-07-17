// Package ontology owns the master ontology data layer: families,
// ontologies, ontology versions, classes, properties, and the polymorphic
// relations between them. It also owns the materialized
// weave_field_ontology_refs table that links project field paths back to
// ontology qnames. This package is a full slice: it owns its store,
// service, and routes, split across three Hosts (AdminHost, APIHost,
// PagesHost) each with its own Mount function rather than a single
// package-level Mount.
//
// Mount point: /admin/ontologies/... (super-admin writes only; reads are
// available to any authenticated user since autocomplete needs them).
//
// The slice owns reads, writes, imports, autocomplete, and path validation.
//
// Sub-packages:
//   - rdf/           — RDF parser + serializer
//   - manifest/      — manifest generation
//   - autocomplete/  — autocomplete + path validation
//   - io/            — import/export, git materializer
//
// Slice composition follows ADR-0001
// (docs/decisions/2026-04-29-weave-module-shapes.md).
package ontology
