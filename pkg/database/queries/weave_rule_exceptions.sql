-- name: WeaveSetRuleException :one
-- Fails on the primary key when the exception exists or has ended: an
-- exception is set once.
INSERT INTO weave_rule_exceptions (project_id, flag, subject, note)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: WeaveEndRuleException :execrows
UPDATE weave_rule_exceptions
SET ended_at = now()
WHERE project_id = $1 AND flag = $2 AND subject = $3 AND ended_at IS NULL;
