-- name: WeaveInsertGraftOverride :exec
INSERT INTO weave_graft_overrides (project_id, model_id, address, is_hidden, max_occurs, display_name)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: WeaveListGraftOverridesByModel :many
SELECT * FROM weave_graft_overrides
WHERE project_id = $1 AND model_id = $2
ORDER BY address;

-- name: WeaveListGraftOverridesByProject :many
SELECT * FROM weave_graft_overrides
WHERE project_id = $1
ORDER BY model_id, address;

-- name: WeaveDeleteGraftOverridesByProject :exec
DELETE FROM weave_graft_overrides WHERE project_id = $1;
