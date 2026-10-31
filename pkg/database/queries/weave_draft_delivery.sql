-- name: WeaveDraftEnrolled :one
SELECT EXISTS(SELECT 1 FROM weave_draft_projects WHERE project_id=$1)::boolean;

-- name: WeaveDraftBaseline :one
SELECT * FROM weave_draft_baselines WHERE project_id=$1;

-- name: WeaveLastDraftDelivery :one
SELECT * FROM weave_draft_deliveries WHERE project_id=$1 ORDER BY revision DESC LIMIT 1;

-- name: WeavePrepareDraftDelivery :one
INSERT INTO weave_draft_deliveries(project_id,revision,snapshot_digest,parent_commit,commit_sha,tree_sha)
VALUES($1,$2,$3,$4,$5,$6) RETURNING *;

-- name: WeaveAcknowledgeDraftDelivery :one
WITH acknowledged AS (
 UPDATE weave_draft_deliveries d SET acknowledged_at=now() WHERE d.project_id=$1 AND d.revision=$2 AND d.commit_sha=$3 AND d.acknowledged_at IS NULL
 RETURNING d.project_id,d.revision,d.commit_sha
), audits AS (
 UPDATE weave_change_set c SET processed_at=now(),git_commit_sha=a.commit_sha,
 materialized_outcome=CASE l.coverage WHEN 'baseline' THEN 'baselined' WHEN 'legacy_coverage' THEN 'covered' ELSE 'committed' END,materialized_error=NULL
 FROM acknowledged a JOIN weave_draft_audits l ON l.project_id=a.project_id AND l.revision=a.revision
 WHERE c.id=l.change_set_id RETURNING c.id
), logs AS (
 UPDATE weave_change_log SET processed_at=now() WHERE change_set_id IN(SELECT id FROM audits) RETURNING id
)
SELECT EXISTS(SELECT 1 FROM acknowledged)::boolean AS acknowledged;

-- name: WeavePendingDraftProjects :many
SELECT p.project_id FROM weave_draft_projects p LEFT JOIN LATERAL
 (SELECT revision,acknowledged_at FROM weave_draft_deliveries d WHERE d.project_id=p.project_id ORDER BY revision DESC LIMIT 1) d ON true
WHERE d.revision IS NULL OR d.acknowledged_at IS NULL OR d.revision<p.revision
ORDER BY COALESCE(d.revision,-1),p.project_id LIMIT $1;

-- name: WeaveDraftCommittedRevision :one
SELECT revision FROM weave_draft_projects WHERE project_id=$1;
