-- name: WeaveCreateProject :one
INSERT INTO weave_projects (
    id, system_name, ui_name, description,
    status, namespace, parent_project_id, owner_id, staging_id, visibility, created_by_id, is_core_weave
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
) RETURNING *;

-- name: WeaveGetProjectByID :one
SELECT * FROM weave_projects WHERE id = $1;

-- name: WeaveUpdateProject :one
UPDATE weave_projects SET
    ui_name = $2, description = $3, system_name = $4, status = $5,
    namespace = $6, parent_project_id = $7, visibility = $8,
    is_core_weave = $9,
    updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: WeaveUpdateProjectAbout :one
UPDATE weave_projects SET
    license = $2, readme = $3, topics = $4, base_url = $5,
    updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: WeaveUpdateProjectConceptListEnforcement :exec
UPDATE weave_projects
SET enforce_concept_lists = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: WeaveGetProjectConceptListEnforcement :one
SELECT enforce_concept_lists
FROM weave_projects
WHERE id = $1;

-- name: WeaveListProjectReleaseVersions :many
SELECT version
FROM weave_releases
WHERE project_id = $1
ORDER BY created_at DESC, version DESC;

-- name: WeaveDeleteProject :exec
DELETE FROM weave_projects WHERE id = $1;

-- name: WeaveListProjects :many
SELECT * FROM weave_projects
WHERE
    (@search::text = ''
     OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
     OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
     OR system_name ILIKE '%' || @search::text || '%')
    AND (@owner_id::text = '' OR owner_id = @owner_id::text)
    AND (@created_by_id::text = '' OR created_by_id = @created_by_id::text)
    AND (@institution_id::text = ''
         OR EXISTS (SELECT 1 FROM weave_project_actors wpa
                    WHERE wpa.project_id = weave_projects.id
                      AND wpa.actor_id = @institution_id::text
                      AND wpa.role = 'owner'))
ORDER BY
    CASE WHEN @sort_by::text = 'ui_name' AND NOT @sort_desc::boolean THEN ui_name->>'en' END ASC NULLS LAST,
    CASE WHEN @sort_by::text = 'ui_name' AND @sort_desc::boolean THEN ui_name->>'en' END DESC NULLS LAST,
    CASE WHEN @sort_by::text = 'system_name' AND NOT @sort_desc::boolean THEN system_name END ASC NULLS LAST,
    CASE WHEN @sort_by::text = 'system_name' AND @sort_desc::boolean THEN system_name END DESC NULLS LAST,
    CASE WHEN @sort_by::text = 'updated_at' AND NOT @sort_desc::boolean THEN updated_at END ASC,
    CASE WHEN @sort_by::text = 'updated_at' AND @sort_desc::boolean THEN updated_at END DESC,
    id
LIMIT @result_limit::integer OFFSET @result_offset::integer;

-- name: WeaveListChildProjects :many
SELECT * FROM weave_projects
WHERE parent_project_id = $1
ORDER BY id ASC;

-- name: WeaveGetProjectParentID :one
-- Returns the parent project's ID (nullable). Used by ResolvedOntologyVersions
-- (parent-walk) and UpdateProjectOntologySettings (cycle detection, Task 15).
SELECT parent_project_id FROM weave_projects WHERE id = $1;

-- name: WeaveCountProjects :one
SELECT COUNT(*) FROM weave_projects
WHERE
    (@search::text = ''
     OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
     OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
     OR system_name ILIKE '%' || @search::text || '%')
    AND (@owner_id::text = '' OR owner_id = @owner_id::text)
    AND (@created_by_id::text = '' OR created_by_id = @created_by_id::text)
    AND (@institution_id::text = ''
         OR EXISTS (SELECT 1 FROM weave_project_actors wpa
                    WHERE wpa.project_id = weave_projects.id
                      AND wpa.actor_id = @institution_id::text
                      AND wpa.role = 'owner'));
