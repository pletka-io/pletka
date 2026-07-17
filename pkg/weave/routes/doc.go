// Package routes declares shared URL-path constants (e.g. the project base
// path) used when building or matching weave routes across slices. This
// package is a shared infrastructure slice: it owns no store or table and is
// imported directly wherever a slice needs the shared path prefix instead of
// hardcoding it.
package routes
