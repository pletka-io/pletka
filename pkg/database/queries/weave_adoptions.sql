-- name: WeaveAdoptionSourcePin :one
-- The link by which the consumer reaches a source: its own parent link, or
-- for an ancestor, the link at the nearest hop. A pinned parent's own links
-- are the ones its pinned release recorded. depth 1 is a direct parent.
WITH RECURSIVE chain AS (
    SELECT wpi.parent_project_id AS project_id, wpi.source_mode,
           COALESCE(wpi.source_version, '')::text AS source_version,
           1 AS depth, ARRAY[wpi.project_id, wpi.parent_project_id] AS path
    FROM weave_project_inheritance wpi
    WHERE wpi.project_id = sqlc.arg(project_id)::text
    UNION ALL
    SELECT n.parent_id, n.source_mode, n.source_version, c.depth + 1, c.path || n.parent_id
    FROM chain c
    JOIN LATERAL (
        SELECT wpi.parent_project_id AS parent_id, wpi.source_mode,
               COALESCE(wpi.source_version, '')::text AS source_version
        FROM weave_project_inheritance wpi
        WHERE c.source_mode = 'draft' AND wpi.project_id = c.project_id
        UNION ALL
        SELECT wpia.parent_project_id, wpia.source_mode,
               COALESCE(wpia.source_version, '')::text
        FROM weave_project_inheritance_archive wpia
        WHERE c.source_mode = 'release' AND wpia.project_id = c.project_id
          AND wpia.version_number = c.source_version
    ) n ON TRUE
    WHERE c.depth < 10 AND NOT (n.parent_id = ANY(c.path))
)
SELECT source_mode, source_version, depth
FROM chain
WHERE project_id = sqlc.arg(parent_project_id)::text
ORDER BY depth
LIMIT 1;

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
