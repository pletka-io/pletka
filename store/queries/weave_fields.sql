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
       OR COALESCE(ontology_path, '') ILIKE '%' || @search::text || '%');

-- name: WeaveListFields :many
SELECT id, created_at, updated_at, semantic_id, system_name, ui_name, description,
       status, project_id, ontology_scope, ontology_path, path_elements,
       expected_value_type, examples, staging_id, deprecated
FROM weave_fields
WHERE project_id = @project_id::text
  AND (@search::text = ''
       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR system_name ILIKE '%' || @search::text || '%'
       OR COALESCE(semantic_id, '') ILIKE '%' || @search::text || '%'
       OR COALESCE(ontology_path, '') ILIKE '%' || @search::text || '%')
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
    staging_id
) VALUES (
    $1, NOW(), NOW(), $2, $3,
    $4, $5, $6, $7,
    $8, $9, $10,
    $11, $12,
    $13
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
    staging_id = EXCLUDED.staging_id
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

-- name: WeaveGetFieldByIdentifier :one
SELECT * FROM weave_fields
WHERE (semantic_id = $1 OR system_name = $1 OR id = $1)
  AND project_id = $2;

-- name: WeaveListFieldsWithCounts :many
SELECT
    f.*,
    (SELECT COUNT(DISTINCT fo.entity_id) FROM weave_field_overrides fo
     WHERE fo.field_id = f.id AND fo.entity_type = 'model') AS model_count,
    (SELECT COUNT(DISTINCT fo.entity_id) FROM weave_field_overrides fo
     WHERE fo.field_id = f.id AND fo.entity_type = 'collection') AS collection_count
FROM weave_fields f
WHERE f.project_id = @project_id::text
ORDER BY f.system_name ASC;

-- name: WeaveGetFieldModels :many
-- Returns models that reference a field via weave_field_overrides
-- (entity_type='model').
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
