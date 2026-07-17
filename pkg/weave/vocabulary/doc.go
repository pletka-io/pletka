// Package vocabulary implements the controlled-vocabulary vertical slice:
// searching, resolving, and linking external vocabulary entries (AAT and
// friends, via pkg/weave/vocabconnector) to fields and collections. This
// package is a full slice: it owns its service, handler, formschema, and
// routes. As a named exception it has no store.go of its own — it delegates
// persistence and lookup to pkg/weave/vocabconnector instead.
package vocabulary
