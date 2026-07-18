// Package mcp is a handler-only module exposing a read-only MCP server at
// /mcp (streamable HTTP, stateless). Authentication is API-key bearer only —
// no session fallback. Tools compose existing slice services through the
// narrow interfaces on Host; capability gating happens inside those
// services via the AuthSnapshot the bearer middleware puts on the context.
// v1 is read-only: no tool mutates anything.
//
// tools_semantic.go imports peer slices (field, model, collection, category,
// vocabulary) for their pure schema builders and view types only — the same
// handler-only-module composition pattern entityschema uses to dispatch to
// per-slice BuildFormSchema/BuildCreateForm, not a peer-slice violation.
package mcp
