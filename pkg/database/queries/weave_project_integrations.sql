-- name: WeaveProjectIntegrationGet :one
SELECT * FROM weave_project_integrations
WHERE project_id = $1 AND integration_id = $2 AND config_id = $3;

-- name: WeaveProjectIntegrationsListForProject :many
SELECT * FROM weave_project_integrations
WHERE project_id = $1
ORDER BY integration_id, config_id;

-- name: WeaveProjectIntegrationsListForIntegration :many
SELECT * FROM weave_project_integrations
WHERE project_id = $1 AND integration_id = $2
ORDER BY config_id;

-- name: WeaveProjectIntegrationsListEnabledForProject :many
SELECT * FROM weave_project_integrations
WHERE project_id = $1 AND enabled = TRUE
ORDER BY integration_id, config_id;

-- name: WeaveProjectIntegrationUpsert :one
INSERT INTO weave_project_integrations
    (project_id, integration_id, config_id, label, enabled, config, secret_ref, managed_instance_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (project_id, integration_id, config_id) DO UPDATE SET
    label               = EXCLUDED.label,
    enabled             = EXCLUDED.enabled,
    config              = EXCLUDED.config,
    secret_ref          = EXCLUDED.secret_ref,
    managed_instance_id = EXCLUDED.managed_instance_id,
    updated_at          = NOW()
RETURNING *;

-- name: WeaveProjectIntegrationDelete :exec
DELETE FROM weave_project_integrations
WHERE project_id = $1 AND integration_id = $2 AND config_id = $3;
