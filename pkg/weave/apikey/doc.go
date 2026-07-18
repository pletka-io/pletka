// Package apikey is the full slice for API keys (table weave_api_keys).
// Keys are minted, listed, and revoked via the `pletka apikey` CLI only in
// v1 — there is no HTTP handler. Verification is consumed by the bearer
// middleware in pkg/auth. Service methods perform no capability checks:
// the only caller is the operator CLI.
package apikey
