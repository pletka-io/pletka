// Package projectpage composes the project overview schema by folding
// together linked ontologies, releases, examples, attributions, and actor
// labels read from other slices. This package is a handler-only module: it
// reads through narrow reader interfaces satisfied by peer slices' services
// and owns no store.
package projectpage
