// Package pages mounts the project shell pages (the server-rendered
// wrapper HTML that hosts the schema-driven islands) via the shared weave
// templates renderer. This package is a handler-only module: it renders
// pages composed from project data read through domain.WeaveStore and owns
// no store of its own.
package pages
