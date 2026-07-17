// Package orgmembers owns organization membership management under
// /orgs/{slug}/members. This package is a full slice: it owns its store,
// service, handler, and routes. Org-capability middleware comes from
// pkg/auth (RequireOrgEdit/RequireOrgRead); it imports no peer slice.
//
// It deliberately mirrors the existing project members slice, but uses:
//   - org capabilities (org.read / org.manage_members)
//   - organization-scoped routes and resources
//   - organization roles: owner, admin, member
package orgmembers
