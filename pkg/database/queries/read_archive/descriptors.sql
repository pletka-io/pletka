-- name: ReleaseDescriptors :one
SELECT document FROM weave_release_descriptors
WHERE project_id=sqlc.arg(project_id)::text AND version_number=sqlc.arg(version)::text;
