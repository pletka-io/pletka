-- name: WeaveCreateModel :one
INSERT INTO weave_models (
    id, system_name, ui_name, description,
    status, project_id, ontology_scope, staging_id, model_type
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: WeaveUpsertModel :one
INSERT INTO weave_models (
    id, system_name, ui_name, description,
    status, project_id, ontology_scope, staging_id, model_type
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (id) DO UPDATE SET
    system_name = EXCLUDED.system_name,
    ui_name = EXCLUDED.ui_name,
    description = EXCLUDED.description,
    status = EXCLUDED.status,
    project_id = EXCLUDED.project_id,
    ontology_scope = EXCLUDED.ontology_scope,
    staging_id = EXCLUDED.staging_id,
    model_type = EXCLUDED.model_type,
    updated_at = NOW()
RETURNING *;

-- name: WeaveGetModelByID :one
SELECT * FROM weave_models WHERE id = $1;

-- name: WeaveUpdateModel :one
UPDATE weave_models SET
    ui_name = $2, description = $3, system_name = $4, status = $5,
    ontology_scope = $6, model_type = $7, updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: WeaveDeleteModel :exec
DELETE FROM weave_models WHERE id = $1;

-- name: WeaveModelIsInUse :one
-- Returns true when any field's override-refs point at this model
-- (ref_type IN ('resource_model','collection_model'), target_id = $1).
-- The model's OWN override rows (entity_type='model', entity_id=$1)
-- are the model's content — they don't count as in-use.
SELECT EXISTS (
    SELECT 1 FROM weave_override_refs r
    JOIN weave_field_overrides o ON o.id = r.override_id
    WHERE r.target_id = $1
      AND r.ref_type IN ('resource_model','collection_model')
      AND o.project_id = $2
    LIMIT 1
)::bool;

-- name: WeaveModelUsageCount :one
-- Count of distinct fields referencing this model as a value target.
SELECT COUNT(DISTINCT o.field_id) AS field_count
FROM weave_override_refs r
JOIN weave_field_overrides o ON o.id = r.override_id
WHERE r.target_id = $1
  AND r.ref_type IN ('resource_model','collection_model')
  AND o.project_id = $2;

-- name: WeaveModelUsageSamples :many
-- Up to 10 sample fields referencing this model as a value target.
SELECT DISTINCT
    f.id          AS field_id,
    f.semantic_id AS field_semantic_id,
    COALESCE(f.ui_name ->> 'en', f.system_name, '') AS field_name
FROM weave_override_refs r
JOIN weave_field_overrides o ON o.id = r.override_id
JOIN weave_fields f ON f.id = o.field_id
WHERE r.target_id = $1
  AND r.ref_type IN ('resource_model','collection_model')
  AND o.project_id = $2
ORDER BY field_name
LIMIT 10;

-- name: WeaveDeprecateModel :exec
UPDATE weave_models
SET deprecated = true, updated_at = NOW()
WHERE id = $1;

-- name: WeaveActivateModel :exec
UPDATE weave_models
SET deprecated = false, updated_at = NOW()
WHERE id = $1;

-- name: WeaveListModels :many
SELECT id, created_at, updated_at, system_name, ui_name, description,
       status, project_id, ontology_scope, staging_id, deprecated, model_type
FROM weave_models
WHERE project_id = @project_id::text
  AND (@search::text = ''
       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR system_name ILIKE '%' || @search::text || '%')
  AND (@model_type::text = '' OR model_type = @model_type::text)
  AND (@scope_class::text = '' OR ((ontology_scope::jsonb->>'prefix') || ':' || (ontology_scope::jsonb->>'local_name')) = @scope_class::text)
ORDER BY
    CASE WHEN model_type = 'core' THEN 0 ELSE 1 END,
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

-- name: WeaveCountModels :one
SELECT COUNT(*) FROM weave_models
WHERE project_id = @project_id::text
  AND (@search::text = ''
       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR system_name ILIKE '%' || @search::text || '%')
  AND (@model_type::text = '' OR model_type = @model_type::text)
  AND (@scope_class::text = '' OR ((ontology_scope::jsonb->>'prefix') || ':' || (ontology_scope::jsonb->>'local_name')) = @scope_class::text);

-- name: WeaveListModelScopeClasses :many
-- Returns distinct ontology scope classes (prefix:local_name) used by
-- models in this project. Drives the scope_class filter dropdown.
SELECT DISTINCT ((ontology_scope::jsonb->>'prefix') || ':' || (ontology_scope::jsonb->>'local_name'))::text AS scope_class
FROM weave_models
WHERE project_id = $1
  AND ontology_scope IS NOT NULL
  AND ontology_scope->>'prefix' IS NOT NULL
  AND ontology_scope->>'local_name' IS NOT NULL
ORDER BY scope_class ASC;

-- name: WeaveGetModelNamesByIDs :many
SELECT id, ui_name FROM weave_models WHERE id = ANY(@ids::text[]);

-- name: WeaveListModelOptions :many
-- Lightweight options list for ref pickers — only the columns the UI needs.
-- No COUNT companion, no description payload, no search filter (the
-- frontend filters client-side once the small list is loaded).
SELECT id, system_name, ui_name, status
FROM weave_models
WHERE project_id = @project_id::text
  AND deprecated = false
ORDER BY system_name ASC;

-- name: WeaveGetModelByIdentifier :one
SELECT * FROM weave_models
WHERE project_id = @project_id::text
  AND (system_name = @identifier::text OR id = @identifier::text)
LIMIT 1;
