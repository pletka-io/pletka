// Package genwiring declares shared generator renderer sets. This package is
// a shared infrastructure slice: it owns no store or table, but pkg/app and
// platform host wiring import it directly to assemble the renderer list for
// pkg/weave/generators rather than each caller re-listing renderers itself.
package genwiring
