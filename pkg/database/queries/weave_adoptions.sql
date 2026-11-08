-- name: WeaveAdoptionSourcePin :one
-- The consumer's parent link to a source: draft-following or a pinned release.
SELECT source_mode, COALESCE(source_version, '')::text AS source_version
FROM weave_project_inheritance
WHERE project_id = sqlc.arg(project_id) AND parent_project_id = sqlc.arg(parent_project_id);

-- name: WeaveReleaseContainsEntity :one
-- Whether a live release of the source holds the entity.
SELECT EXISTS (
    SELECT 1 FROM weave_releases r
    WHERE r.project_id = sqlc.arg(source_project_id)::text
      AND r.version = sqlc.arg(version)::text
      AND r.archived_at IS NULL
      AND (
        (sqlc.arg(entity_type)::text = 'field' AND EXISTS (SELECT 1 FROM weave_fields_archive a WHERE a.project_id = r.project_id AND a.version_number = r.version AND a.id = sqlc.arg(entity_id)::text))
     OR (sqlc.arg(entity_type)::text = 'model' AND EXISTS (SELECT 1 FROM weave_models_archive a WHERE a.project_id = r.project_id AND a.version_number = r.version AND a.id = sqlc.arg(entity_id)::text))
     OR (sqlc.arg(entity_type)::text = 'collection' AND EXISTS (SELECT 1 FROM weave_collections_archive a WHERE a.project_id = r.project_id AND a.version_number = r.version AND a.id = sqlc.arg(entity_id)::text))
     OR (sqlc.arg(entity_type)::text = 'category' AND EXISTS (SELECT 1 FROM weave_categories_archive a WHERE a.project_id = r.project_id AND a.version_number = r.version AND a.id = sqlc.arg(entity_id)::text))
      )
)::boolean AS contains;

-- name: WeaveLatestReleaseContaining :one
-- The newest live release of the source that holds the entity.
SELECT r.version
FROM weave_releases r
WHERE r.project_id = sqlc.arg(source_project_id)::text
  AND r.archived_at IS NULL
  AND (
    (sqlc.arg(entity_type)::text = 'field' AND EXISTS (SELECT 1 FROM weave_fields_archive a WHERE a.project_id = r.project_id AND a.version_number = r.version AND a.id = sqlc.arg(entity_id)::text))
 OR (sqlc.arg(entity_type)::text = 'model' AND EXISTS (SELECT 1 FROM weave_models_archive a WHERE a.project_id = r.project_id AND a.version_number = r.version AND a.id = sqlc.arg(entity_id)::text))
 OR (sqlc.arg(entity_type)::text = 'collection' AND EXISTS (SELECT 1 FROM weave_collections_archive a WHERE a.project_id = r.project_id AND a.version_number = r.version AND a.id = sqlc.arg(entity_id)::text))
 OR (sqlc.arg(entity_type)::text = 'category' AND EXISTS (SELECT 1 FROM weave_categories_archive a WHERE a.project_id = r.project_id AND a.version_number = r.version AND a.id = sqlc.arg(entity_id)::text))
  )
ORDER BY r.created_at DESC
LIMIT 1;

-- name: WeaveDraftFollowingAdoptionSources :many
-- Sources the project's receipts still read at their draft. A receipt left by
-- a deleted model or collection has no context to re-save and is not counted.
SELECT DISTINCT a.source_project_id
FROM weave_adoptions a
WHERE a.project_id = sqlc.arg(project_id) AND COALESCE(a.source_version, '') = ''
  AND (a.context_entity_type NOT IN ('model', 'collection')
    OR (a.context_entity_type = 'model' AND EXISTS (SELECT 1 FROM weave_models m WHERE m.id = a.context_entity_id))
    OR (a.context_entity_type = 'collection' AND EXISTS (SELECT 1 FROM weave_collections c WHERE c.id = a.context_entity_id)))
ORDER BY a.source_project_id;
