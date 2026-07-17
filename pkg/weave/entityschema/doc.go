// Package entityschema dispatches list- and form-schema requests by entity
// type for pages that need generic, schema-driven CRUD across multiple weave
// entities. This package is a handler-only module: per the dispatcher rule,
// it delegates to each entity slice's own BuildXSchema functions rather than
// reimplementing schemas, and it owns no store.
package entityschema
