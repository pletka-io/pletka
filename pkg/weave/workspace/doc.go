// Package workspace owns the user-facing workspace page shells.
//
// It is a handler-only slice: no table ownership, no direct sqlc usage.
// It composes organization and project services into /profile, /orgs,
// /orgs/{slug}, and /orgs/{slug}/settings.
package workspace
