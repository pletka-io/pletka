// Package vocabconnector defines the Connector interface and Entry type
// that back external controlled-vocabulary lookups (AAT, CSV, local, and
// SPARQL sources implemented in its subpackages). This package is a shared infrastructure
// slice: the vocabulary slice has no store.go of its own and imports
// vocabconnector directly as its persistence/lookup layer instead of going
// through a reader interface.
package vocabconnector
