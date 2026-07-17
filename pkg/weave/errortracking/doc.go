// Package errortracking captures server-side request failures and
// client-side JavaScript exceptions in a single weave_error_events
// table. It is the production observability surface used while users test the
// app and still helps expose missing routes as a queryable migration backlog.
// This package is a full slice: it owns its store, service, handler, and
// routes for the weave_error_events table.
//
// Server side: a chi middleware wraps every request, writes a row
// for any 4xx/5xx response, and redacts sensitive headers/body
// fields before persisting. 2xx/3xx responses are skipped.
//
// Client side: POST /errors/client accepts a small JSON envelope
// from the browser and stores it in the same table with source =
// 'client'. The frontend wrapper installs window.onerror and
// onunhandledrejection handlers that call this endpoint.
//
// A small admin-only viewer mounts at /admin/errors and renders
// recent events with filters on status, route, source, and actor.
package errortracking
