// Package generators contains the weave-native generator foundation. This
// package is a shared infrastructure slice: it owns no store or table and
// is imported directly by visualization, genwiring, and CLI commands
// rather than through a peer reader interface.
//
// Generators consume resolved snapshots built from domain.WeaveStore. They
// should not query legacy model structures or infer ontology semantics from
// naming conventions; ontology authority lives in pkg/weave/ontology.
package generators
