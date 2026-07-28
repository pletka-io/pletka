-- Override PK is bigserial (auto-generated). No id parameter on insert.
-- All entity references use semantic IDs directly.

-- name: WeaveCreateOverride :one
INSERT INTO weave_field_overrides (
    field_id, project_id, entity_type, entity_id,
    position, collection_order,
    display_name, description, collection_name,
    category_id, part_of_collection_id,
    expected_value_type, set_value,
    is_required, min_occurs, max_occurs, is_hidden, visibility,
    staging_id, content_hash
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
) RETURNING *;

-- name: WeaveUpsertBaseOverride :one
-- Upsert a base override (entity_type=''). Matches on (field_id, project_id)
-- via the partial unique index uniq_wfo_base_field_project.
INSERT INTO weave_field_overrides (
    field_id, project_id, entity_type, entity_id,
    position, collection_order,
    display_name, description, collection_name,
    category_id, part_of_collection_id,
    expected_value_type, set_value,
    is_required, min_occurs, max_occurs, is_hidden, visibility,
    staging_id, content_hash
) VALUES (
    $1, $2, '', '', $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
)
ON CONFLICT (field_id, project_id) WHERE entity_type = '' DO UPDATE SET
    position = EXCLUDED.position,
    collection_order = EXCLUDED.collection_order,
    display_name = EXCLUDED.display_name,
    description = EXCLUDED.description,
    collection_name = EXCLUDED.collection_name,
    category_id = EXCLUDED.category_id,
    part_of_collection_id = EXCLUDED.part_of_collection_id,
    expected_value_type = EXCLUDED.expected_value_type,
    set_value = EXCLUDED.set_value,
    is_required = EXCLUDED.is_required,
    min_occurs = EXCLUDED.min_occurs,
    max_occurs = EXCLUDED.max_occurs,
    is_hidden = EXCLUDED.is_hidden,
    visibility = EXCLUDED.visibility,
    staging_id = EXCLUDED.staging_id,
    content_hash = EXCLUDED.content_hash,
    updated_at = NOW()
RETURNING *;

-- name: WeaveGetOverrideByID :one
SELECT * FROM weave_field_overrides WHERE id = $1;

-- name: WeaveGetBaseOverride :one
SELECT * FROM weave_field_overrides
WHERE field_id = $1 AND project_id = $2 AND entity_type = ''
LIMIT 1;

-- name: WeaveUpdateOverride :one
UPDATE weave_field_overrides SET
    position = $2, collection_order = $3,
    display_name = $4, description = $5, collection_name = $6,
    category_id = $7, part_of_collection_id = $8,
    expected_value_type = $9, set_value = $10,
    is_required = $11, min_occurs = $12, max_occurs = $13,
    is_hidden = $14, visibility = $15,
    content_hash = $16,
    updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: WeaveDeleteOverride :exec
DELETE FROM weave_field_overrides WHERE id = $1;

-- name: WeaveDeleteOverridesForEntity :exec
DELETE FROM weave_field_overrides WHERE entity_type = $1 AND entity_id = $2;

-- name: WeaveListOverridesForEntity :many
SELECT * FROM weave_field_overrides
WHERE entity_type = $1 AND entity_id = $2
ORDER BY position;

-- name: WeaveListOverridesForOwner :many
-- gitmaterializer scopedRewrite(): overrides placed on a single model/
-- collection owner, scoped to the project. Drives writeOverridesForOwner,
-- which regenerates one owner's overrides/ subtree after a scoped rewrite
-- (mirrors WeaveListOverridesByProjectAndType, narrowed to one entity_id).
SELECT * FROM weave_field_overrides
WHERE project_id = $1 AND entity_type = $2 AND entity_id = $3
ORDER BY position;

-- name: WeaveListOverridesForField :many
SELECT * FROM weave_field_overrides WHERE field_id = $1
ORDER BY CASE entity_type WHEN 'model' THEN 0 WHEN 'collection' THEN 1 ELSE 2 END, position;

-- name: WeaveCountOverridesForEntity :one
SELECT COUNT(*) FROM weave_field_overrides WHERE entity_type = $1 AND entity_id = $2;

-- name: WeaveResolveForModel :many
-- Returns ALL overrides for a model with field identity joined.
-- No DISTINCT ON — every override is shown (copy-on-adopt).
-- Group by category → collection, order by position.
SELECT
    fo.id, fo.field_id, fo.project_id, fo.entity_type, fo.entity_id, fo.position, fo.collection_order,
    fo.display_name, fo.description, fo.collection_name, fo.category_id, fo.part_of_collection_id,
    fo.set_value, fo.is_required, fo.min_occurs, fo.max_occurs, fo.is_hidden, fo.visibility,
    fo.staging_id, fo.created_at, fo.updated_at, fo.content_hash,
    f.semantic_id    AS field_semantic_id,
    f.system_name    AS field_system_name,
    f.ontology_scope AS field_ontology_scope,
    f.ontology_path  AS field_ontology_path,
    f.expected_value_type AS field_expected_value_type,
    f.path_elements  AS field_path_elements,
    f.subfield_paths AS field_subfield_paths
FROM weave_field_overrides fo
JOIN weave_fields f ON f.id = fo.field_id
WHERE fo.entity_type = 'model'
  AND fo.entity_id = @model_id::text
  AND fo.project_id = @project_id::text
ORDER BY fo.category_id, fo.part_of_collection_id, fo.position;

-- name: WeaveResolveForCollection :many
-- Returns ALL overrides for a collection with field identity joined.
SELECT
    fo.id, fo.field_id, fo.project_id, fo.entity_type, fo.entity_id, fo.position, fo.collection_order,
    fo.display_name, fo.description, fo.collection_name, fo.category_id, fo.part_of_collection_id,
    fo.set_value, fo.is_required, fo.min_occurs, fo.max_occurs, fo.is_hidden, fo.visibility,
    fo.staging_id, fo.created_at, fo.updated_at, fo.content_hash,
    f.semantic_id    AS field_semantic_id,
    f.system_name    AS field_system_name,
    f.ontology_scope AS field_ontology_scope,
    f.ontology_path  AS field_ontology_path,
    f.expected_value_type AS field_expected_value_type,
    f.path_elements  AS field_path_elements,
    f.subfield_paths AS field_subfield_paths
FROM weave_field_overrides fo
JOIN weave_fields f ON f.id = fo.field_id
WHERE fo.entity_type = 'collection'
  AND fo.entity_id = @collection_id::text
  AND fo.project_id = @project_id::text
ORDER BY fo.category_id, fo.position;

-- name: WeaveCreateOverrideRef :exec
INSERT INTO weave_override_refs (override_id, ref_type, target_id, semantic_id, position)
VALUES ($1, $2, $3, $4, $5);

-- name: WeaveDeleteOverrideRefs :exec
DELETE FROM weave_override_refs WHERE override_id = $1;

-- name: WeaveListOverrideRefs :many
SELECT * FROM weave_override_refs WHERE override_id = $1 ORDER BY ref_type, position;

-- name: WeaveListRefsForOverrides :many
SELECT * FROM weave_override_refs WHERE override_id = ANY(@override_ids::bigint[])
ORDER BY override_id, ref_type, position;

-- name: WeaveListBaseOverridesForFields :many
-- Returns base overrides (entity_type='') for a set of field IDs scoped
-- to a project. Used by the resolver to enforce the "base SetValue is
-- non-overridable" rule when materialising model/collection views.
SELECT * FROM weave_field_overrides
WHERE entity_type = ''
  AND project_id = $1
  AND field_id = ANY($2::text[]);

-- name: WeaveListBaseFieldCategoriesForProject :many
-- Distinct categories assigned to base field overrides in a project.
-- Drives the category filter dropdown on the field list.
SELECT DISTINCT c.id, c.ui_name, c.canonical_order
FROM weave_categories c
JOIN weave_field_overrides fo ON fo.category_id = c.id
WHERE fo.project_id = $1
  AND fo.entity_type = ''
  AND COALESCE(fo.category_id, '') <> ''
ORDER BY c.canonical_order ASC, c.id ASC;

-- name: WeaveListAllBaseFieldCategoryAssignments :many
-- Returns (field_id, category_id) for every base override in the project.
-- Drives the category filter + category column on the field list.
SELECT field_id, category_id
FROM weave_field_overrides
WHERE project_id = $1
  AND entity_type = ''
  AND COALESCE(category_id, '') <> '';

-- name: WeaveListOverridesByProjectAndType :many
-- Returns all overrides for a project filtered by entity_type.
-- Used by CSV export to get all model/collection/base overrides at once.
SELECT * FROM weave_field_overrides
WHERE project_id = $1 AND entity_type = $2
ORDER BY entity_id, position;

-- name: WeaveCountFieldUsage :one
-- "How many models / collections in this project use this field?"
-- Drives the field item view's Stats tab. Counts
-- distinct entity_ids per kind from weave_field_overrides — that
-- table encodes both model_fields and collection_fields junctions
-- via (entity_type, entity_id). override_count is the total number
-- of model/collection override rows referencing this field (the
-- override-row count, not the distinct entity count).
SELECT
    count(DISTINCT entity_id) FILTER (WHERE entity_type = 'model')::bigint
        AS models_using,
    count(DISTINCT entity_id) FILTER (WHERE entity_type = 'collection')::bigint
        AS collections_using,
    count(*) FILTER (WHERE entity_type IN ('model', 'collection'))::bigint
        AS override_count
FROM weave_field_overrides
WHERE field_id = $1 AND project_id = $2;

-- name: WeaveClosurePlacementsForField :many
-- gitmaterializer closure(): models/collections in this project that place
-- fieldID via an override row. Editing/deleting the field must also rewrite
-- each placing owner's overrides/ subtree.
SELECT DISTINCT entity_type, entity_id
FROM weave_field_overrides
WHERE field_id = $1 AND project_id = $2 AND entity_type IN ('model', 'collection');

-- name: WeaveClosureOverrideOwner :one
-- gitmaterializer closure(): resolves a change_log "override" entry's
-- entity_id (a weave_field_overrides.id) to its owning model/collection, or
-- (entity_type '', field_id) for a base override. Scoped by project_id so
-- the within-project invariant is self-enforcing.
SELECT entity_type, entity_id, field_id
FROM weave_field_overrides
WHERE id = $1 AND project_id = $2;
