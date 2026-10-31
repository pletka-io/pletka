-- name: WeavePrepareProjectImport :one
SELECT weave_prepare_project_import(sqlc.arg(payload)::jsonb)::boolean AS created;

-- name: WeaveImportProjectRelease :one
SELECT weave_import_project_release(sqlc.arg(payload)::jsonb)::boolean AS created;

-- name: WeaveRestoreProjectWorkspace :one
SELECT weave_restore_project_workspace(sqlc.arg(payload)::jsonb)::bigint AS detached_staging_refs;

-- name: WeaveProjectImportLockTimeout :one
SELECT current_setting('lock_timeout')::text AS value;

-- name: WeaveSetProjectImportLockTimeout :exec
SELECT set_config('lock_timeout', sqlc.arg(value)::text, false);
