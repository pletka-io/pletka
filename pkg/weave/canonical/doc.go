// Package canonical produces byte-identical YAML serialization of weave
// entities for git materialization, change_log payloads, and content hashing.
// This package is a shared infrastructure slice: it owns no store and is
// imported directly by entity slices (category, collection, field, model)
// for canonical serialization instead of each slice reimplementing it.
package canonical
