-- name: WeaveMembershipUpsert :exec
INSERT INTO weave_memberships (actor_id, scope_type, scope_id, role)
VALUES ($1, $2, $3, $4)
ON CONFLICT (actor_id, scope_type, scope_id)
DO UPDATE SET role = EXCLUDED.role;

-- name: WeaveMembershipDelete :exec
DELETE FROM weave_memberships
WHERE actor_id = $1 AND scope_type = $2 AND scope_id = $3;

-- name: WeaveMembershipListByActor :many
SELECT actor_id, scope_type, scope_id, role, created_at
FROM weave_memberships
WHERE actor_id = $1;

-- name: WeaveMembershipListByScope :many
SELECT m.actor_id, m.scope_type, m.scope_id, m.role, m.created_at,
       wa.slug, wa.display_name
FROM weave_memberships m
JOIN weave_actors wa ON wa.id = m.actor_id
WHERE m.scope_type = $1 AND m.scope_id = $2;

-- name: WeaveMembershipListProjectMembers :many
-- Members slice list query. Returns explicit memberships UNION the
-- synthesised project-owner row when the owner has no explicit
-- membership (defence-in-depth — explicit rows land at create time
-- via Service.Create + the 030 backfill migration). The is_owner flag
-- lets the frontend hide the Remove action and lets the service-layer
-- guard reject delete attempts.
SELECT actor_id, role, slug, display_name, email, is_owner
FROM (
    SELECT m.actor_id, m.role, wa.slug, wa.display_name,
           COALESCE(wa.email, '')::text AS email,
           (m.actor_id = p.owner_id) AS is_owner
    FROM weave_memberships m
    JOIN weave_actors wa ON wa.id = m.actor_id
    JOIN weave_projects p ON p.id = m.scope_id
    WHERE m.scope_type = 'project' AND m.scope_id = $1

    UNION ALL

    SELECT p.owner_id AS actor_id,
           'owner'::text AS role,
           wa.slug, wa.display_name,
           COALESCE(wa.email, '')::text AS email,
           true AS is_owner
    FROM weave_projects p
    JOIN weave_actors wa ON wa.id = p.owner_id
    WHERE p.id = $1 AND p.owner_id <> ''
      AND NOT EXISTS (
          SELECT 1 FROM weave_memberships m2
          WHERE m2.actor_id = p.owner_id
            AND m2.scope_type = 'project'
            AND m2.scope_id = p.id
      )
) combined
ORDER BY is_owner DESC, display_name ASC;

-- name: WeaveProjectOwnerID :one
-- Returns the owner_id of a project. Used by the members slice's
-- Service.Remove to reject deletion of the owner's membership.
SELECT owner_id FROM weave_projects WHERE id = $1;

-- name: WeaveMembershipFindActorByEmailOrSlug :one
-- Resolves a user-typed identifier (email or slug) to an actor row.
-- Returns no rows when nothing matches.
SELECT wa.id AS actor_id, wa.slug, wa.display_name,
       COALESCE(wa.email, '')::text AS email
FROM weave_actors wa
WHERE wa.slug = $1 OR wa.email = $1
LIMIT 1;

-- name: WeaveMembershipListAvailableActors :many
-- Returns actors not yet members of the project (and not the project
-- owner). Feeds the Add Member form's autocomplete select. Cap at 500
-- — small populations today; revisit when actor counts grow.
SELECT wa.id AS actor_id, wa.slug, wa.display_name,
       COALESCE(wa.email, '')::text AS email
FROM weave_actors wa
WHERE NOT EXISTS (
    SELECT 1 FROM weave_memberships m
    WHERE m.actor_id = wa.id
      AND m.scope_type = 'project'
      AND m.scope_id = $1
)
AND NOT EXISTS (
    SELECT 1 FROM weave_projects p
    WHERE p.id = $1 AND p.owner_id = wa.id
)
ORDER BY wa.display_name ASC
LIMIT 500;

-- name: WeaveMembershipSnapshotForActor :many
-- Feeds BuildSnapshot: actor-level role (for super_admin detection)
-- + memberships + projects the actor directly owns. The three row kinds
-- unify into one query so the middleware makes a single round-trip.
SELECT 'actor'::text AS kind, ''::text AS scope_type, ''::text AS scope_id, role
FROM weave_actors
WHERE weave_actors.id = $1
UNION ALL
SELECT 'membership'::text AS kind, scope_type, scope_id, role
FROM weave_memberships
WHERE actor_id = $1
UNION ALL
SELECT 'owned'::text AS kind, 'project'::text AS scope_type, weave_projects.id AS scope_id, 'owner'::text AS role
FROM weave_projects
WHERE weave_projects.owner_id = $1;
