// Package weave is the core domain layer for Pletka's semantic patterns:
// Fields, Models, and Collections, together with categories, projects, and
// the override chain that ties them together.
//
// This package depends on nothing else inside Pletka. The HTTP layer in
// server/, the persistence layer (when added), and the gitsync layer all
// import weave; never the reverse.
package weave
