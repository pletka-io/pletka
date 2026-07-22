-- name: WeaveGetFieldByID :one
SELECT * FROM weave_fields WHERE id = $1;

-- name: WeaveCountFields :one
SELECT COUNT(*) FROM weave_fields
WHERE project_id = @project_id::text
  AND (@search::text = ''
       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR system_name ILIKE '%' || @search::text || '%'
       OR COALESCE(semantic_id, '') ILIKE '%' || @search::text || '%'
       OR COALESCE(ontology_path, '') ILIKE '%' || @search::text || '%')
  AND (@status::text = '' OR status = @status::text)
  AND (@owner_id::text = '' OR EXISTS
       (SELECT 1 FROM weave_field_overrides fo WHERE fo.field_id =
       weave_fields.id AND fo.entity_type IN ('model','collection') AND
       fo.entity_id = @owner_id));

-- name: WeaveListFields :many
SELECT id, created_at, updated_at, semantic_id, system_name, ui_name, description,
       status, project_id, ontology_scope, ontology_path, path_elements,
       expected_value_type, examples, staging_id, deprecated, subfield_paths
FROM weave_fields
WHERE project_id = @project_id::text
  AND (@search::text = ''
       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR system_name ILIKE '%' || @search::text || '%'
       OR COALESCE(semantic_id, '') ILIKE '%' || @search::text || '%'
       OR COALESCE(ontology_path, '') ILIKE '%' || @search::text || '%')
  AND (@status::text = '' OR status = @status::text)
  AND (@owner_id::text = '' OR EXISTS
       (SELECT 1 FROM weave_field_overrides fo WHERE fo.field_id =
       weave_fields.id AND fo.entity_type IN ('model','collection') AND
       fo.entity_id = @owner_id))
ORDER BY
    CASE WHEN @sort_by::text = 'ui_name' AND NOT @sort_desc::boolean THEN ui_name->>'en' END ASC NULLS LAST,
    CASE WHEN @sort_by::text = 'ui_name' AND @sort_desc::boolean THEN ui_name->>'en' END DESC NULLS LAST,
    CASE WHEN @sort_by::text = 'name' AND NOT @sort_desc::boolean THEN system_name END ASC NULLS LAST,
    CASE WHEN @sort_by::text = 'name' AND @sort_desc::boolean THEN system_name END DESC NULLS LAST,
    CASE WHEN @sort_by::text = 'system_name' AND NOT @sort_desc::boolean THEN system_name END ASC NULLS LAST,
    CASE WHEN @sort_by::text = 'system_name' AND @sort_desc::boolean THEN system_name END DESC NULLS LAST,
    CASE WHEN @sort_by::text = 'updated_at' AND NOT @sort_desc::boolean THEN updated_at END ASC,
    CASE WHEN @sort_by::text = 'updated_at' AND @sort_desc::boolean THEN updated_at END DESC,
    system_name ASC
LIMIT @result_limit::integer OFFSET @result_offset::integer;

-- name: WeaveCreateField :one
INSERT INTO weave_fields (
    id, created_at, updated_at, semantic_id, system_name,
    ui_name, description, status, project_id,
    ontology_scope, ontology_path, path_elements,
    expected_value_type, examples,
    staging_id
) VALUES (
    $1, NOW(), NOW(), $2, $3,
    $4, $5, $6, $7,
    $8, $9, $10,
    $11, $12,
    $13
) RETURNING *;

-- name: WeaveUpsertField :one
INSERT INTO weave_fields (
    id, created_at, updated_at, semantic_id, system_name,
    ui_name, description, status, project_id,
    ontology_scope, ontology_path, path_elements,
    expected_value_type, examples,
    staging_id, subfield_paths
) VALUES (
    $1, NOW(), NOW(), $2, $3,
    $4, $5, $6, $7,
    $8, $9, $10,
    $11, $12,
    $13, $14
)
ON CONFLICT (id) DO UPDATE SET
    updated_at = NOW(),
    semantic_id = EXCLUDED.semantic_id,
    system_name = EXCLUDED.system_name,
    ui_name = EXCLUDED.ui_name,
    description = EXCLUDED.description,
    status = EXCLUDED.status,
    project_id = EXCLUDED.project_id,
    ontology_scope = EXCLUDED.ontology_scope,
    ontology_path = EXCLUDED.ontology_path,
    path_elements = EXCLUDED.path_elements,
    expected_value_type = EXCLUDED.expected_value_type,
    examples = EXCLUDED.examples,
    staging_id = EXCLUDED.staging_id,
    subfield_paths = EXCLUDED.subfield_paths
RETURNING *;

-- name: WeaveUpdateField :one
UPDATE weave_fields SET
    updated_at = NOW(),
    ui_name = $2,
    description = $3,
    system_name = $4,
    status = $5,
    ontology_scope = $6,
    ontology_path = $7,
    path_elements = $8,
    expected_value_type = $9,
    examples = $10
WHERE id = $1
RETURNING *;

-- name: WeaveDeleteField :exec
DELETE FROM weave_fields WHERE id = $1;

-- name: WeaveListReceiptFieldDirect :many
-- Direct (depth-1) field references from the receipt seed — every
-- field used by an override attached directly to the seed entity.
-- For a model seed this is the model's own field set; for a
-- collection seed it's the collection's field set; for a field seed
-- it's the field itself. Pairs with WeaveListReceiptFieldClosure for
-- the direct-vs-transitive count split.
SELECT DISTINCT field_id
FROM weave_field_overrides
WHERE entity_type = $2::text
  AND entity_id = $1::text
  AND COALESCE(field_id, '') <> '';

-- name: WeaveListReceiptFieldClosure :many
-- Returns every field used by any override anywhere in the
-- model+collection transitive closure rooted at the receipt seed.
-- Drives the per-receipt bill-of-materials. The closure walks the
-- override-ref graph (same shape as the model/collection closure);
-- the final SELECT pulls field_id out of every override row in the
-- walked set.
--
-- For a seed of kind 'field', the walker can't recurse (fields don't
-- have override refs), so the closure is empty here — that edge case
-- is handled at the handler level by short-circuiting with the seed
-- field itself.
WITH RECURSIVE walk(kind, id) AS (
    SELECT $2::text AS kind, $1::text AS id
  UNION
    SELECT nxt.kind, nxt.id
    FROM walk w
    JOIN weave_field_overrides o ON o.entity_type = w.kind AND o.entity_id = w.id
    JOIN weave_override_refs r ON r.override_id = o.id
    CROSS JOIN LATERAL (
      SELECT 'model'::text AS kind, m.id
      FROM weave_models m
      WHERE m.id = r.target_id AND r.ref_type = 'resource_model'
      UNION ALL
      SELECT 'collection'::text, c.id
      FROM weave_collections c
      WHERE c.id = r.target_id AND r.ref_type = 'collection_model'
    ) nxt
)
SELECT DISTINCT o.field_id
FROM walk w
JOIN weave_field_overrides o ON o.entity_type = w.kind AND o.entity_id = w.id
WHERE COALESCE(o.field_id, '') <> ''
ORDER BY o.field_id;

-- name: WeaveListReferenceAdoptedFields :many
-- Returns every field from another project that the current project's
-- field overrides reference via field_id. Drives the
-- "Adopted (by reference)" slice of the field list rule (adopt/adapt
-- rollout plan, task 3b). Only counts overrides whose entity_type
-- belongs to the current project (model or collection overrides
-- locally authored).
SELECT DISTINCT f.*
FROM weave_fields f
JOIN weave_field_overrides o ON o.field_id = f.id
WHERE o.project_id = $1
  AND f.project_id <> $1
ORDER BY f.id;

-- name: WeaveGetFieldByIdentifier :one
SELECT * FROM weave_fields
WHERE (semantic_id = $1 OR system_name = $1 OR id = $1)
  AND project_id = $2;

-- name: WeaveGetFieldByGlobalIdentifier :one
-- Project-agnostic lookup for identifiers that are globally unique
-- (semantic_id encodes the owning project prefix; id is a ULID). Used by
-- restore to resolve a cross-project override reference (e.g. a vendored
-- parent's override pointing at another vendored project's field). System
-- names are project-scoped and deliberately excluded.
SELECT * FROM weave_fields
WHERE semantic_id = $1 OR id = $1;

-- name: WeaveGetFieldModels :many
-- Returns models that reference a field via weave_field_overrides
-- (entity_type='model'). Replaces the legacy model_fields junction.
SELECT
    m.id                                              AS model_id,
    COALESCE(m.ui_name ->> 'en', m.system_name, '')   AS model_name
FROM weave_field_overrides fo
JOIN weave_models m ON m.id = fo.entity_id
WHERE fo.entity_type = 'model' AND fo.field_id = $1
ORDER BY model_name;

-- name: WeaveGetFieldCollections :many
-- Returns collections that reference a field via weave_field_overrides
-- (entity_type='collection').
SELECT
    col.id                                                AS collection_id,
    COALESCE(col.ui_name ->> 'en', col.system_name, '')   AS collection_name
FROM weave_field_overrides fo
JOIN weave_collections col ON col.id = fo.entity_id
WHERE fo.entity_type = 'collection' AND fo.field_id = $1
ORDER BY collection_name;

-- name: WeaveFieldIsInUse :one
-- Returns true when the field is referenced by any non-base override
-- (entity_type IN ('model','collection')). The base override row
-- (entity_type='') is the field's own intrinsic state and does NOT
-- count as in-use. Hot path: delete preflight + UI gating.
SELECT EXISTS (
    SELECT 1 FROM weave_field_overrides
    WHERE field_id = $1
      AND project_id = $2
      AND entity_type IN ('model', 'collection')
    LIMIT 1
)::bool;

-- name: WeaveFieldUsageModels :many
-- Returns up to 10 sample models that reference this field. Used for
-- the 409 payload when delete is blocked.
SELECT
    m.id                                              AS model_id,
    m.system_name                                     AS model_system_name,
    COALESCE(m.ui_name ->> 'en', m.system_name, '')   AS model_name
FROM weave_field_overrides fo
JOIN weave_models m ON m.id = fo.entity_id
WHERE fo.entity_type = 'model'
  AND fo.field_id = $1
  AND fo.project_id = $2
ORDER BY model_name
LIMIT 10;

-- name: WeaveFieldUsageCollections :many
-- Returns up to 10 sample collections that reference this field.
SELECT
    col.id                                                AS collection_id,
    col.system_name                                       AS collection_system_name,
    COALESCE(col.ui_name ->> 'en', col.system_name, '')   AS collection_name
FROM weave_field_overrides fo
JOIN weave_collections col ON col.id = fo.entity_id
WHERE fo.entity_type = 'collection'
  AND fo.field_id = $1
  AND fo.project_id = $2
ORDER BY collection_name
LIMIT 10;

-- name: WeaveFieldUsageCounts :one
-- Returns model + collection adoption counts for usage stats display.
SELECT
    COUNT(*) FILTER (WHERE entity_type = 'model')      AS model_count,
    COUNT(*) FILTER (WHERE entity_type = 'collection') AS collection_count
FROM weave_field_overrides
WHERE field_id = $1
  AND project_id = $2
  AND entity_type IN ('model', 'collection');

-- name: WeaveDeprecateField :exec
-- Soft-retires a field. Existing references stay intact; pickers stop
-- offering it for new connections.
UPDATE weave_fields
SET deprecated = true, updated_at = NOW()
WHERE id = $1;

-- name: WeaveActivateField :exec
-- Reverses Deprecate. The field becomes available for new connections
-- again.
UPDATE weave_fields
SET deprecated = false, updated_at = NOW()
WHERE id = $1;
