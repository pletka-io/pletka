// Package actorlabels resolves actor IDs to display names for slices that
// need to label actor references (e.g. "created by", "last edited by")
// without owning actor data themselves. This package is a shared infrastructure
// slice: like override, it is imported directly by consuming slices instead
// of through a per-slice reader interface.
package actorlabels
