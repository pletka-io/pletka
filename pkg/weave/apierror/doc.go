// Package apierror is the canonical wire shape + constructor surface
// for every JSON error response in the weave HTTP API. This package is a
// shared infrastructure slice: it owns no table, but every slice handler
// imports it directly for error responses instead of building its own
// envelope.
//
// One source of truth — handlers no longer build map[string]any
// envelopes inline, and slices no longer carry their own
// writeError / writeValidationErrors helpers. The wire shape stays
// backwards-compatible with what FormRenderer and the existing
// frontend already expect:
//
//	{
//	  "code": "validation",            // optional, machine-readable
//	  "error": "validation error",     // human summary
//	  "message": "Please correct...",  // optional longer help
//	  "errors": {"field": ["msg"]}     // FormRenderer per-field
//	}
//
// New code should use the constructors (BadRequest, Validation, etc.)
// and the Write helper. Old code keeps working — slice handlers
// forward their writeError/writeValidationErrors helpers to apierror
// during the migration.
package apierror
