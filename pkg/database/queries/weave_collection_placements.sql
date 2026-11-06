-- Collection placements: per-(model, category, collection) constraints for a
-- collection group inside a model. PK is bigserial; all
-- entity references use semantic IDs directly.

-- name: WeaveUpsertCollectionPlacement :one
INSERT INTO weave_collection_placements (
    project_id, model_id, category_id, collection_id,
    is_required, min_occurs, max_occurs, is_hidden
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (model_id, category_id, collection_id) DO UPDATE SET
    is_required = EXCLUDED.is_required,
    min_occurs  = EXCLUDED.min_occurs,
    max_occurs  = EXCLUDED.max_occurs,
    is_hidden   = EXCLUDED.is_hidden,
    updated_at  = now()
RETURNING *;

-- name: WeaveListCollectionPlacementsByModel :many
SELECT * FROM weave_collection_placements
WHERE model_id = $1
ORDER BY category_id, collection_id;

-- name: WeaveGetCollectionPlacement :one
SELECT * FROM weave_collection_placements
WHERE model_id = $1 AND category_id = $2 AND collection_id = $3;

-- name: WeaveDeleteCollectionPlacement :exec
DELETE FROM weave_collection_placements
WHERE model_id = $1 AND category_id = $2 AND collection_id = $3;

-- name: WeaveDeleteCollectionPlacementsByModel :exec
DELETE FROM weave_collection_placements
WHERE model_id = $1;

-- name: WeaveListPlacementsAtVersion :many
-- Placements at a release. Empty model_id means every model in the project.
SELECT id, project_id, model_id, category_id, collection_id,
       is_required, min_occurs, max_occurs, is_hidden,
       created_at, updated_at, version_number
FROM weave_collection_placements_archive
WHERE project_id = @project_id::text
  AND version_number = @version_number::text
  AND (@model_id::text = '' OR model_id = @model_id::text)
ORDER BY model_id, category_id, collection_id;

-- name: WeaveListCollectionPlacementsByProject :many
SELECT * FROM weave_collection_placements
WHERE project_id = $1
ORDER BY model_id, category_id, collection_id;

-- name: WeaveDeleteCollectionPlacementsByProject :exec
DELETE FROM weave_collection_placements
WHERE project_id = $1;

-- name: WeaveListPlacedCollectionPlacementsByProject :many
-- Placements whose model places a field of the collection through a
-- model-scoped override: the rows a project tree may carry.
SELECT p.* FROM weave_collection_placements p
WHERE p.project_id = $1
  AND EXISTS (
    SELECT 1 FROM weave_field_overrides o
    WHERE o.project_id = p.project_id
      AND o.entity_type = 'model'
      AND o.entity_id = p.model_id
      AND o.part_of_collection_id = p.collection_id
  )
ORDER BY p.model_id, p.category_id, p.collection_id;

-- name: WeaveRestoreCollectionPlacement :one
-- Hydration's upsert: unlike WeaveUpsertCollectionPlacement it never takes
-- over a row of another project sharing the unique key (no row returned).
INSERT INTO weave_collection_placements (
    project_id, model_id, category_id, collection_id,
    is_required, min_occurs, max_occurs, is_hidden
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (model_id, category_id, collection_id) DO UPDATE SET
    is_required = EXCLUDED.is_required,
    min_occurs  = EXCLUDED.min_occurs,
    max_occurs  = EXCLUDED.max_occurs,
    is_hidden   = EXCLUDED.is_hidden,
    updated_at  = now()
WHERE weave_collection_placements.project_id = EXCLUDED.project_id
RETURNING id;
