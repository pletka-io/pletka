// Package members owns the project Members settings pane. This package is
// a full slice: it owns its store, service, handler, and routes.
//
// Mount root: /projects/{projectID}/members. The slice exposes the
// list of actors with explicit project memberships (rows in
// weave_memberships), plus the operations to add a member, change a
// member's role, or remove a member.
//
// Routes:
//
//	GET    /                           → List members (joined with actor info)
//	GET    /list-schema                → ListSchema for ListManager
//	GET    /form-schema                → Add-member form schema
//	POST   /                           → Add member (resolves email/slug → actor)
//	PATCH  /{actorID}                  → Change member role
//	DELETE /{actorID}                  → Remove member
//
// Roles: project membership uses the four-level project role rank
// (owner > maintainer > contributor > viewer) defined in pkg/auth.
//
// Permission gates: read requires project.read; write requires
// project.manage_members. The auth.WithProjectResource middleware in
// pkg/auth populates the per-request Resource so the gate fires
// against the project's actual visibility.
//
// Out of scope:
//   - Inviting brand-new actors. The Add endpoint resolves an existing
//     actor by email or slug; if none matches it errors with 422. New
//     actor registration goes through the auth signup flow elsewhere.
//   - Org-level memberships. This slice is project-scoped only.
package members
