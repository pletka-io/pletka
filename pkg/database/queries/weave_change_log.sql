-- name: WeaveCreateChangeSet :one
INSERT INTO weave_change_set (
    project_id, actor_id, actor_name, actor_email, commit_message
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: WeaveCloseChangeSet :exec
UPDATE weave_change_set SET closed_at = now() WHERE id = $1;

-- name: WeaveGetChangeSet :one
SELECT * FROM weave_change_set WHERE id = $1;

-- name: WeaveListUnprocessedChangeSets :many
SELECT * FROM weave_change_set
WHERE closed_at IS NOT NULL AND processed_at IS NULL
ORDER BY closed_at ASC
LIMIT $1;

-- name: WeaveMarkChangeSetProcessed :exec
UPDATE weave_change_set
SET processed_at = now(),
    git_commit_sha = $2,
    materialized_outcome = $3,
    materialized_duration_ms = $4,
    materialized_files = $5,
    materialized_changed = $6
WHERE id = $1;

-- name: WeaveRecordMaterializationFailure :exec
-- Records a failed materialization attempt without marking the change set
-- processed, so the poller retries it. processed_at stays NULL.
UPDATE weave_change_set
SET materialized_outcome = 'failed',
    materialized_error = $2,
    materialized_duration_ms = $3
WHERE id = $1;

-- name: WeaveCreateChangeLogEntry :one
INSERT INTO weave_change_log (
    change_set_id, entity_type, entity_id, operation, project_id,
    file_path, payload, previous_payload
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: WeaveListChangeLogForChangeSet :many
SELECT * FROM weave_change_log
WHERE change_set_id = $1
ORDER BY id ASC;

-- name: WeaveMarkChangeLogProcessed :exec
UPDATE weave_change_log
SET processed_at = now()
WHERE change_set_id = $1;
