// Package apikey is the full slice for API keys (table weave_api_keys).
// Keys are minted, listed, and revoked via the `pletka apikey` CLI, and via
// the actor-scoped HTTP surface mounted at /me/api-keys (list, list-schema,
// form-schema, create, revoke — see routes.go). Verification is consumed by
// the bearer middleware in pkg/auth. Service methods perform no capability
// checks: HTTP handlers hard-scope every operation to the authenticated
// caller's actor ID instead.
package apikey
