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
