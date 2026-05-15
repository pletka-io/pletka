-- name: StagingInsert :exec
INSERT INTO weave_import_staging (import_run, source_type, source_path, project_name, semantic_id, airtable_ref, raw_data)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: StagingListPending :many
SELECT * FROM weave_import_staging
WHERE import_run = $1 AND status = 'pending' AND source_type = $2
ORDER BY id;

-- name: StagingUpdateStatus :exec
UPDATE weave_import_staging SET status = $2, error_msg = $3 WHERE id = $1;

-- name: StagingBuildIndex :many
SELECT id, source_type, project_name, semantic_id, airtable_ref
FROM weave_import_staging
WHERE import_run = $1 AND semantic_id IS NOT NULL AND semantic_id != '';

-- name: StagingCountByStatus :many
SELECT source_type, status, COUNT(*) AS count
FROM weave_import_staging
WHERE import_run = $1
GROUP BY source_type, status
ORDER BY source_type, status;

-- name: StagingDeleteRun :exec
DELETE FROM weave_import_staging WHERE import_run = $1;

-- name: StagingLookupByRef :one
SELECT id, semantic_id FROM weave_import_staging
WHERE airtable_ref = $1 AND import_run = $2
LIMIT 1;

-- name: StagingLookupBySemanticID :one
SELECT id, airtable_ref FROM weave_import_staging
WHERE semantic_id = $1 AND source_type = $2 AND import_run = $3
LIMIT 1;

-- name: StagingListBySourcePath :many
SELECT * FROM weave_import_staging
WHERE source_path = $1
ORDER BY id;

-- name: StagingListByRun :many
SELECT * FROM weave_import_staging
WHERE import_run = $1 AND source_type = $2
ORDER BY id;

-- name: StagingListAll :many
SELECT * FROM weave_import_staging
WHERE import_run = $1
ORDER BY id;

-- name: StagingEnrich :exec
UPDATE weave_import_staging
SET normalized_id = @normalized_id,
    path_elements = @path_elements,
    ontology_scope = @ontology_scope,
    project_id = @project_id
WHERE id = @id;

-- name: StagingPopulateProjectIDs :exec
-- Bulk-set project_id from weave_projects.id using system_name = project_name.
UPDATE weave_import_staging s
SET project_id = p.id
FROM weave_projects p
WHERE p.system_name = s.project_name
  AND s.import_run = @import_run
  AND s.project_id IS NULL;

-- name: StagingSetNormalizedID :exec
UPDATE weave_import_staging SET normalized_id = $2 WHERE id = $1;

-- name: StagingCountByNormalized :many
SELECT source_type,
       COUNT(*)::int AS total,
       COUNT(normalized_id)::int AS normalized,
       (COUNT(*) - COUNT(normalized_id))::int AS missing
FROM weave_import_staging
WHERE import_run = $1
GROUP BY source_type
ORDER BY source_type;

-- name: StagingValidateRefs :many
SELECT id, source_type, project_name, semantic_id, normalized_id, error_msg
FROM weave_import_staging
WHERE import_run = @import_run AND status = 'ref_invalid';

-- name: StagingResolveByRef :one
-- Resolve an airtable recID to the normalized_id (entity PK), scoped to a project.
SELECT normalized_id FROM weave_import_staging
WHERE airtable_ref = sqlc.arg(airtable_ref) AND import_run = sqlc.arg(import_run)
  AND project_id = sqlc.arg(project_id)
  AND normalized_id IS NOT NULL
LIMIT 1;

-- name: StagingResolveByRefGlobal :one
-- Resolve an airtable recID to the normalized_id (entity PK), across all projects.
-- Fallback when project-scoped resolution fails (cross-project refs).
SELECT normalized_id FROM weave_import_staging
WHERE airtable_ref = sqlc.arg(airtable_ref) AND import_run = sqlc.arg(import_run)
  AND normalized_id IS NOT NULL
LIMIT 1;

-- name: StagingResolveBySemanticID :one
-- Resolve a semantic_id to the normalized_id (entity PK), scoped to a project.
SELECT normalized_id FROM weave_import_staging
WHERE import_run = sqlc.arg(import_run) AND normalized_id IS NOT NULL
  AND project_id = sqlc.arg(project_id)
  AND (semantic_id = sqlc.arg(ref) OR normalized_id = sqlc.arg(ref))
LIMIT 1;

-- name: StagingResolveBySemanticIDGlobal :one
-- Resolve a semantic_id to the normalized_id (entity PK), across all projects.
-- Fallback when project-scoped resolution fails.
SELECT normalized_id FROM weave_import_staging
WHERE import_run = sqlc.arg(import_run) AND normalized_id IS NOT NULL
  AND (semantic_id = sqlc.arg(ref) OR normalized_id = sqlc.arg(ref))
LIMIT 1;
