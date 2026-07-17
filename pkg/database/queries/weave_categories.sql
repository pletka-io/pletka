-- name: WeaveGetCategoryByID :one
SELECT * FROM weave_categories WHERE id = $1;

-- name: WeaveListCategories :many
SELECT * FROM weave_categories
WHERE project_id = $1
ORDER BY canonical_order ASC, system_name ASC;

-- name: WeaveCountCategories :one
SELECT COUNT(*) FROM weave_categories WHERE project_id = $1;

-- name: WeaveCreateCategory :one
INSERT INTO weave_categories (
    id, created_at, updated_at, semantic_id, system_name,
    ui_name, description, status, project_id, canonical_order
) VALUES (
    $1, NOW(), NOW(), $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: WeaveUpsertCategory :one
INSERT INTO weave_categories (
    id, created_at, updated_at, semantic_id, system_name,
    ui_name, description, status, project_id, canonical_order
) VALUES (
    $1, NOW(), NOW(), $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (id) DO UPDATE SET
    updated_at = NOW(),
    semantic_id = EXCLUDED.semantic_id,
    system_name = EXCLUDED.system_name,
    ui_name = EXCLUDED.ui_name,
    description = EXCLUDED.description,
    status = EXCLUDED.status,
    project_id = EXCLUDED.project_id,
    canonical_order = EXCLUDED.canonical_order
RETURNING *;

-- name: WeaveUpdateCategory :one
UPDATE weave_categories SET
    updated_at = NOW(),
    ui_name = $2,
    description = $3,
    system_name = $4,
    status = $5,
    canonical_order = $6
WHERE id = $1
RETURNING *;

-- name: WeaveDeleteCategory :exec
DELETE FROM weave_categories WHERE id = $1;

-- name: WeaveUpdateCategoryOrder :exec
UPDATE weave_categories
SET canonical_order = $2, updated_at = NOW()
WHERE id = $1;

-- name: WeaveListCategoriesWithCounts :many
-- Returns each category in the project with its usage counts and a fast
-- in_use boolean. Field-level category lives on the base override
-- (entity_type='') after migration 027; in_use accepts either ULID or
-- semantic_id matches in any override row (base, model, collection).
SELECT
    c.*,
    (SELECT COUNT(*) FROM weave_field_overrides
     WHERE entity_type = ''
       AND category_id = c.id
       AND project_id = @project_id::text) as field_count,
    (SELECT COUNT(*) FROM weave_field_overrides
     WHERE entity_type = 'model'
       AND category_id = c.id
       AND project_id = @project_id::text) as model_field_count,
    (SELECT COUNT(*) FROM weave_field_overrides
     WHERE entity_type = 'collection'
       AND category_id = c.id
       AND project_id = @project_id::text) as collection_field_count,
    EXISTS (
        SELECT 1 FROM weave_field_overrides fo
        WHERE fo.category_id != ''
          AND fo.category_id IN (c.id, c.semantic_id)
          AND fo.project_id = @project_id::text
    ) as in_use
FROM weave_categories c
WHERE c.project_id = @project_id::text
ORDER BY c.canonical_order ASC, c.system_name ASC;

-- name: WeaveGetCategoryByIdentifier :one
SELECT * FROM weave_categories
WHERE (semantic_id = $1 OR system_name = $1 OR id = $1)
  AND project_id = $2;

-- name: WeaveReassignFieldsToCategory :exec
-- Reassigns field-level category from $1 to $2 within the project.
-- After migration 027, field category lives on the base override
-- (entity_type='') — this updates that row.
UPDATE weave_field_overrides
SET category_id = $2
WHERE category_id = $1
  AND entity_type = ''
  AND project_id = $3;

-- name: WeaveClearFieldOverrideCategory :exec
-- Clears category_id on any field override (base, model, collection) that
-- references the given category. Used when a category is deleted.
UPDATE weave_field_overrides
SET category_id = ''
WHERE category_id = $1;

-- name: WeaveCategoryModelFieldOverrides :many
-- Returns model-scoped field overrides that reference a given category,
-- joined with model name and field name for display.
SELECT
    fo.id                                              AS override_id,
    COALESCE(m.ui_name ->> 'en', m.system_name, '')   AS model_name,
    COALESCE(f.ui_name ->> 'en', f.system_name, '')   AS field_name
FROM weave_field_overrides fo
JOIN weave_models m ON fo.entity_id = m.id
JOIN weave_fields f ON fo.field_id = f.id
WHERE fo.entity_type = 'model'
  AND fo.category_id = $1
  AND m.project_id = $2;

-- name: WeaveCategoryCollectionFieldOverrides :many
-- Returns collection-scoped field overrides that reference a given category,
-- joined with collection name and field name for display.
SELECT
    fo.id                                                AS override_id,
    COALESCE(col.ui_name ->> 'en', col.system_name, '') AS collection_name,
    COALESCE(f.ui_name ->> 'en', f.system_name, '')     AS field_name
FROM weave_field_overrides fo
JOIN weave_collections col ON fo.entity_id = col.id
JOIN weave_fields f ON fo.field_id = f.id
WHERE fo.entity_type = 'collection'
  AND fo.category_id = $1
  AND col.project_id = $2;

-- name: WeaveDeprecateCategory :exec
-- Soft-retires a category. Existing references stay intact; pickers stop
-- offering it for new connections.
UPDATE weave_categories
SET deprecated = true, updated_at = NOW()
WHERE id = $1;

-- name: WeaveActivateCategory :exec
-- Reverses Deprecate. The category becomes available for new connections
-- again.
UPDATE weave_categories
SET deprecated = false, updated_at = NOW()
WHERE id = $1;

-- name: WeaveCategoryIsInUse :one
-- Returns true if any field override references this category by ULID or
-- semantic_id, across all entity_type values (base, model, collection).
-- Hot path: used both server-side (delete preflight) and exposed in list
-- responses to gate UI delete buttons.
SELECT EXISTS (
    SELECT 1 FROM weave_field_overrides
    WHERE category_id != ''
      AND category_id IN (sqlc.arg(category_ulid)::text, sqlc.arg(category_semantic_id)::text)
) AS in_use;
