-- name: WeaveLockDraftCapture :exec
SELECT weave_lock_draft_capture($1::text);

-- name: WeaveUnlockDraftCapture :one
SELECT pg_advisory_unlock(weave_draft_lock_key($1::text)) AND pg_advisory_unlock_shared(731,421) AND pg_advisory_unlock_shared(hashtext('project:'||$1::text)) AS unlocked;

-- name: WeaveStoreDraftSnapshot :one
SELECT weave_store_draft_snapshot($1::jsonb)::text AS digest;

-- name: WeaveDraftSnapshotFiles :many
SELECT e.key::text AS path,b.body FROM weave_draft_snapshots s
CROSS JOIN LATERAL jsonb_each_text(s.manifest) e JOIN weave_draft_blobs b ON b.digest=e.value
WHERE s.digest=$1 ORDER BY 1;

-- name: WeaveEnrollDraft :exec
INSERT INTO weave_draft_projects(project_id,snapshot_digest) VALUES($1,$2);

-- name: WeaveDraftState :one
SELECT * FROM weave_draft_projects WHERE project_id=$1 FOR UPDATE;

-- name: WeaveStartDraftMutation :one
INSERT INTO weave_draft_mutations(project_id,revision,purpose,actor_id,actor_name,actor_email,message,before_digest)
SELECT p.project_id,p.revision+1,$2,$3,$4,$5,$6,p.snapshot_digest FROM weave_draft_projects p WHERE p.project_id=$1
RETURNING *;

-- name: WeaveSealDraftMutation :execrows
UPDATE weave_draft_mutations SET after_digest=$3 WHERE project_id=$1 AND revision=$2 AND transaction_id=pg_current_xact_id() AND after_digest IS NULL;

-- name: WeaveAdvanceDraftState :execrows
UPDATE weave_draft_projects SET revision=$2,snapshot_digest=$3 WHERE project_id=$1 AND revision=$2-1;

-- name: WeaveDraftMutation :one
SELECT * FROM weave_draft_mutations WHERE project_id=$1 AND revision=$2 AND after_digest IS NOT NULL;

-- name: WeaveDraftEffects :many
SELECT * FROM weave_draft_effects WHERE project_id=$1 AND revision=$2 ORDER BY id;

-- name: WeaveDraftSnapshotSize :one
SELECT count(*)::bigint AS files,COALESCE(sum(octet_length(b.body)),0)::bigint AS bytes
FROM weave_draft_snapshots s CROSS JOIN LATERAL jsonb_each_text(s.manifest) e
JOIN weave_draft_blobs b ON b.digest=e.value WHERE s.digest=$1;

-- name: WeaveLockEntityInTransaction :exec
SELECT weave_lock_entity_in_transaction(sqlc.arg(project_key)::text,sqlc.arg(entity_key)::text,sqlc.arg(budget_ms)::bigint);
