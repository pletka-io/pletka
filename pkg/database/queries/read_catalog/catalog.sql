-- name: ProjectReleaseCatalog :many
SELECT r.project_id, r.version, r.title, r.description, r.created_by_id,
 CASE WHEN isfinite(r.created_at) THEN to_char(r.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM r.created_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE r.created_at::text END::text AS created_at,
 CASE WHEN r.archived_at IS NULL THEN '' WHEN isfinite(r.archived_at) THEN to_char(r.archived_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || CASE WHEN EXTRACT(YEAR FROM r.archived_at AT TIME ZONE 'UTC') < 0 THEN ' BC' ELSE '' END ELSE r.archived_at::text END::text AS archived_at,
 r.archived_message
FROM weave_releases r
WHERE r.project_id=sqlc.arg(project_id)::text
  AND (sqlc.arg(selected_version)::text='' OR r.version=sqlc.arg(selected_version)::text)
ORDER BY r.version COLLATE "C";
