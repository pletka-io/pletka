-- name: WeaveAuthCreate :one
INSERT INTO weave_auth (actor_id, password_hash, email_verified_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: WeaveAuthGetByActorID :one
SELECT * FROM weave_auth WHERE actor_id = $1;

-- name: WeaveAuthGetByEmail :one
-- Case-insensitive: SSO (Zitadel) and Pletka may disagree on email casing
-- for the same address, so the join normalizes both sides. Used only by
-- oidcauth's callback lookup — GetByEmailOrSlug (password login) stays
-- case-sensitive on purpose, see that query below.
SELECT a.*, wa.email, wa.slug, wa.display_name, wa.role AS actor_role
FROM weave_auth a
JOIN weave_actors wa ON wa.id = a.actor_id
WHERE LOWER(wa.email) = LOWER(sqlc.arg(email));

-- name: WeaveAuthGetByEmailOrSlug :one
-- Used by Login: accepts either the user's email or their slug (username).
SELECT a.*, wa.email, wa.slug, wa.display_name, wa.role AS actor_role
FROM weave_auth a
JOIN weave_actors wa ON wa.id = a.actor_id
WHERE wa.email = $1 OR wa.slug = $1;

-- name: WeaveAuthUpdatePassword :exec
UPDATE weave_auth
SET password_hash = $2, updated_at = NOW()
WHERE actor_id = $1;

-- name: WeaveAuthMarkLogin :exec
UPDATE weave_auth
SET last_login_at = NOW(), updated_at = NOW()
WHERE actor_id = $1;

-- name: WeaveAuthSetResetToken :exec
UPDATE weave_auth
SET password_reset_token = $2,
    password_reset_expires_at = $3,
    updated_at = NOW()
WHERE actor_id = $1;

-- name: WeaveAuthGetByResetToken :one
SELECT * FROM weave_auth
WHERE password_reset_token = $1
  AND password_reset_expires_at > NOW();

-- name: WeaveAuthClearResetToken :exec
UPDATE weave_auth
SET password_reset_token = NULL,
    password_reset_expires_at = NULL,
    updated_at = NOW()
WHERE actor_id = $1;

-- name: WeaveAuthDelete :exec
DELETE FROM weave_auth WHERE actor_id = $1;

-- name: WeaveAuthBumpPermsVersion :exec
UPDATE weave_auth
SET perms_version = perms_version + 1, updated_at = NOW()
WHERE actor_id = $1;

-- name: WeaveActorCreatePerson :one
INSERT INTO weave_actors (id, type, display_name, system_name, slug, email)
VALUES ($1, 'person', $2, $3, $4, $5)
RETURNING id, display_name, slug, email;

-- name: WeaveAuthGetByActorIDWithActor :one
SELECT a.*, wa.email, wa.slug, wa.display_name, wa.role AS actor_role
FROM weave_auth a
JOIN weave_actors wa ON wa.id = a.actor_id
WHERE a.actor_id = $1;
