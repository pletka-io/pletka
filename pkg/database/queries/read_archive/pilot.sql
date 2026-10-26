-- name: GetCollection :one
SELECT * FROM weave_collections_archive WHERE project_id = @project_id::text AND id = @id::text AND version_number = @version::text;

-- name: ListCollections :many
SELECT id, created_at, updated_at, system_name, ui_name, description,
       status, project_id, ontology_scope, collection_number,
       canonical_collection_order, staging_id, deprecated, default_category_id, version_number
FROM weave_collections_archive
WHERE project_id = @project_id::text AND version_number = @version::text
  AND (@search::text = ''
       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR system_name ILIKE '%' || @search::text || '%')
  AND (@scope_class::text = '' OR ((ontology_scope::jsonb->>'prefix') || ':' || (ontology_scope::jsonb->>'local_name')) = ANY(string_to_array(@scope_class::text, ',')))
  AND (@category_id::text = '' OR default_category_id = ANY(string_to_array(@category_id::text, ',')))
  AND (@status::text = '' OR status = @status::text)
ORDER BY
    CASE WHEN @sort_by::text = 'ui_name' AND NOT @sort_desc::boolean THEN ui_name->>'en' END ASC NULLS LAST,
    CASE WHEN @sort_by::text = 'ui_name' AND @sort_desc::boolean THEN ui_name->>'en' END DESC NULLS LAST,
    CASE WHEN @sort_by::text = 'name' AND NOT @sort_desc::boolean THEN system_name END ASC NULLS LAST,
    CASE WHEN @sort_by::text = 'name' AND @sort_desc::boolean THEN system_name END DESC NULLS LAST,
    CASE WHEN @sort_by::text = 'system_name' AND NOT @sort_desc::boolean THEN system_name END ASC NULLS LAST,
    CASE WHEN @sort_by::text = 'system_name' AND @sort_desc::boolean THEN system_name END DESC NULLS LAST,
    CASE WHEN @sort_by::text = 'updated_at' AND NOT @sort_desc::boolean THEN updated_at END ASC,
    CASE WHEN @sort_by::text = 'updated_at' AND @sort_desc::boolean THEN updated_at END DESC,
    canonical_collection_order ASC, system_name ASC
LIMIT @result_limit::integer OFFSET @result_offset::integer;

-- name: CountCollections :one
SELECT COUNT(*) FROM weave_collections_archive
WHERE project_id = @project_id::text AND version_number = @version::text
  AND (@search::text = ''
       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR system_name ILIKE '%' || @search::text || '%')
  AND (@scope_class::text = '' OR ((ontology_scope::jsonb->>'prefix') || ':' || (ontology_scope::jsonb->>'local_name')) = ANY(string_to_array(@scope_class::text, ',')))
  AND (@category_id::text = '' OR default_category_id = ANY(string_to_array(@category_id::text, ',')))
  AND (@status::text = '' OR status = @status::text);

-- name: CollectionScopeClasses :many
SELECT DISTINCT ((ontology_scope::jsonb->>'prefix') || ':' || (ontology_scope::jsonb->>'local_name'))::text AS scope_class
FROM weave_collections_archive
WHERE project_id = @project_id::text AND version_number = @version::text
  AND ontology_scope IS NOT NULL
  AND ontology_scope->>'prefix' IS NOT NULL
  AND ontology_scope->>'local_name' IS NOT NULL
ORDER BY scope_class ASC;

-- name: CollectionCategories :many
SELECT id, ui_name, canonical_order FROM weave_categories_archive WHERE project_id = @project_id::text AND version_number = @version::text ORDER BY canonical_order, id;

-- name: CollectionCounts :many
SELECT entity_id, count(*) AS field_count FROM weave_field_overrides_archive WHERE project_id = @project_id::text AND version_number = @version::text AND entity_type = 'collection' AND entity_id = ANY(@ids::text[]) GROUP BY entity_id;

-- name: CollectionUsageCount :one
SELECT COUNT(DISTINCT o.field_id) AS field_count
FROM weave_override_refs_archive r
JOIN weave_field_overrides_archive o ON o.id = r.override_id AND r.version_number = o.version_number AND r.project_id = o.project_id
WHERE o.project_id = @project_id::text AND o.version_number = @version::text AND r.target_id = @id::text AND r.ref_type IN ('resource_model', 'collection_model');

-- name: CollectionUsageSamples :many
SELECT DISTINCT o.field_id,
COALESCE(f.semantic_id, o.field_id)::text AS field_semantic_id,
COALESCE(f.ui_name->>'en', f.system_name, o.field_id)::text AS field_name
FROM weave_override_refs_archive r
JOIN weave_field_overrides_archive o ON o.id = r.override_id AND r.version_number = o.version_number AND r.project_id = o.project_id
LEFT JOIN weave_fields_archive f ON f.id = o.field_id AND f.project_id = o.project_id AND f.version_number = o.version_number
WHERE o.project_id = @project_id::text AND o.version_number = @version::text AND r.target_id = @id::text AND r.ref_type IN ('resource_model', 'collection_model')
ORDER BY field_name, field_id LIMIT 10;

-- name: GetOverride :one
SELECT * FROM weave_field_overrides_archive WHERE project_id = @project_id::text AND id = @id::bigint AND version_number = @version::text;

-- name: ListEntityOverrides :many
SELECT * FROM weave_field_overrides_archive WHERE project_id = @project_id::text AND version_number = @version::text AND entity_type = @entity_type::text AND entity_id = @entity_id::text ORDER BY position, id;

-- name: ListFieldOverrides :many
SELECT * FROM weave_field_overrides_archive WHERE project_id = @project_id::text AND version_number = @version::text AND field_id = @field_id::text ORDER BY CASE entity_type WHEN 'model' THEN 0 WHEN 'collection' THEN 1 ELSE 2 END, position, id;

-- name: ListTypeOverrides :many
SELECT * FROM weave_field_overrides_archive WHERE project_id = @project_id::text AND version_number = @version::text AND entity_type = @entity_type::text ORDER BY entity_id, position, id;

-- name: GetBaseOverride :one
SELECT * FROM weave_field_overrides_archive WHERE project_id = @project_id::text AND version_number = @version::text AND field_id = @field_id::text AND entity_type = '' LIMIT 1;

-- name: OverrideRefs :many
SELECT r.* FROM weave_override_refs_archive r JOIN weave_field_overrides_archive o ON o.id = r.override_id AND r.version_number = o.version_number AND r.project_id = o.project_id
WHERE o.project_id = @project_id::text AND o.version_number = @version::text AND o.id = @id::bigint ORDER BY r.ref_type, r.position;

-- name: ModelPlacements :many
SELECT * FROM weave_collection_placements_archive WHERE project_id = @project_id::text AND version_number = @version::text AND model_id = @model_id::text ORDER BY category_id, collection_id;
