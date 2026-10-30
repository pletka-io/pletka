-- name: SourceProjectAccess :one
-- Current access policy only, not released project content.
SELECT id, owner_id, visibility FROM weave_projects WHERE id = @project_id::text;
