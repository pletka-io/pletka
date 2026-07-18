-- name: WeaveAPIKeyCreate :one
INSERT INTO weave_api_keys (id, actor_id, name, key_hash, key_prefix, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: WeaveAPIKeyGetByHash :one
SELECT * FROM weave_api_keys WHERE key_hash = $1;

-- name: WeaveAPIKeyList :many
SELECT * FROM weave_api_keys ORDER BY created_at DESC;

-- name: WeaveAPIKeyListByActor :many
SELECT * FROM weave_api_keys WHERE actor_id = $1 ORDER BY created_at DESC;

-- name: WeaveAPIKeyTouch :exec
UPDATE weave_api_keys SET last_used_at = now() WHERE id = $1;

-- name: WeaveAPIKeyRevoke :execrows
UPDATE weave_api_keys
SET revoked_at = now()
WHERE (id = $1 OR key_prefix = $1) AND revoked_at IS NULL;
