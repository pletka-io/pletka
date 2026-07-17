-- name: WeaveCreateCollection :one
INSERT INTO weave_collections (
    id, system_name, ui_name, description,
    status, project_id, ontology_scope,
    collection_number, canonical_collection_order,
    default_category_id, staging_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
) RETURNING *;

-- name: WeaveUpsertCollection :one
INSERT INTO weave_collections (
    id, system_name, ui_name, description,
    status, project_id, ontology_scope,
    collection_number, canonical_collection_order,
    default_category_id, staging_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
ON CONFLICT (id) DO UPDATE SET
    system_name = EXCLUDED.system_name,
    ui_name = EXCLUDED.ui_name,
    description = EXCLUDED.description,
    status = EXCLUDED.status,
    project_id = EXCLUDED.project_id,
    ontology_scope = EXCLUDED.ontology_scope,
    collection_number = EXCLUDED.collection_number,
    canonical_collection_order = EXCLUDED.canonical_collection_order,
    default_category_id = EXCLUDED.default_category_id,
    staging_id = EXCLUDED.staging_id,
    updated_at = NOW()
RETURNING *;

-- name: WeaveGetCollectionByID :one
SELECT * FROM weave_collections WHERE id = $1;

-- name: WeaveUpdateCollection :one
UPDATE weave_collections SET
    ui_name = $2, description = $3, system_name = $4, status = $5,
    ontology_scope = $6, collection_number = $7, canonical_collection_order = $8,
    default_category_id = $9,
    updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: WeaveDeleteCollection :exec
DELETE FROM weave_collections WHERE id = $1;

-- name: WeaveCollectionIsInUse :one
-- Returns true when any field's override-refs point at this
-- collection (ref_type IN ('resource_model','collection_model'),
-- target_id = $1). The collection's OWN override rows
-- (entity_type='collection', entity_id=$1) are the collection's
-- content — they don't count.
SELECT EXISTS (
    SELECT 1 FROM weave_override_refs r
    JOIN weave_field_overrides o ON o.id = r.override_id
    WHERE r.target_id = $1
      AND r.ref_type IN ('resource_model','collection_model')
      AND o.project_id = $2
    LIMIT 1
)::bool;

-- name: WeaveCollectionUsageCount :one
-- Count of distinct fields referencing this collection as a value target.
SELECT COUNT(DISTINCT o.field_id) AS field_count
FROM weave_override_refs r
JOIN weave_field_overrides o ON o.id = r.override_id
WHERE r.target_id = $1
  AND r.ref_type IN ('resource_model','collection_model')
  AND o.project_id = $2;

-- name: WeaveCollectionUsageSamples :many
-- Up to 10 sample fields referencing this collection as a value target.
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

-- name: WeaveDeprecateCollection :exec
UPDATE weave_collections
SET deprecated = true, updated_at = NOW()
WHERE id = $1;

-- name: WeaveActivateCollection :exec
UPDATE weave_collections
SET deprecated = false, updated_at = NOW()
WHERE id = $1;

-- name: WeaveListCollections :many
SELECT id, created_at, updated_at, system_name, ui_name, description,
       status, project_id, ontology_scope, collection_number,
       canonical_collection_order, staging_id, deprecated, default_category_id
FROM weave_collections
WHERE project_id = @project_id::text
  AND (@search::text = ''
       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR system_name ILIKE '%' || @search::text || '%')
  AND (@scope_class::text = '' OR ((ontology_scope::jsonb->>'prefix') || ':' || (ontology_scope::jsonb->>'local_name')) = ANY(string_to_array(@scope_class::text, ',')))
  AND (@category_id::text = '' OR default_category_id = ANY(string_to_array(@category_id::text, ',')))
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

-- name: WeaveCountCollections :one
SELECT COUNT(*) FROM weave_collections
WHERE project_id = @project_id::text
  AND (@search::text = ''
       OR EXISTS (SELECT 1 FROM jsonb_each_text(ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR EXISTS (SELECT 1 FROM jsonb_each_text(description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR system_name ILIKE '%' || @search::text || '%')
  AND (@scope_class::text = '' OR ((ontology_scope::jsonb->>'prefix') || ':' || (ontology_scope::jsonb->>'local_name')) = ANY(string_to_array(@scope_class::text, ',')))
  AND (@category_id::text = '' OR default_category_id = ANY(string_to_array(@category_id::text, ',')));

-- name: WeaveListReceiptCollectionDirect :many
-- Direct (depth-1) collection references from the receipt seed —
-- every collection that the seed's overrides reference via a
-- collection_model override ref. Pairs with
-- WeaveListReceiptCollectionClosure (transitive). Both queries follow
-- the same edge type (override refs) so direct ≤ transitive holds.
-- part_of_collection_id is a separate relationship on the override
-- row and is intentionally excluded — the closure walker doesn't
-- traverse it either.
SELECT DISTINCT c.id
FROM weave_field_overrides o
JOIN weave_override_refs r ON r.override_id = o.id AND r.ref_type = 'collection_model'
JOIN weave_collections c ON c.id = r.target_id
WHERE o.entity_type = $2::text AND o.entity_id = $1::text;

-- name: WeaveListReceiptCollectionClosure :many
-- Returns every collection in the transitive closure rooted at the
-- receipt seed (source_entity_id + entity_type). Drives the
-- per-receipt bill-of-materials on the Adoptions tab. Walker pattern
-- mirrors the model closure — both kinds traverse the same
-- override-ref graph, only the final filter differs.
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
SELECT DISTINCT id FROM walk WHERE kind = 'collection';

-- name: WeaveListReferenceAdoptedCollections :many
-- Returns every collection from another project that the current
-- project's field overrides reference via part_of_collection_id.
-- Drives the "Adopted (by reference)" slice of the collection list
-- rule (adopt/adapt rollout plan, task 3b).
SELECT DISTINCT c.*
FROM weave_collections c
JOIN weave_field_overrides o ON o.part_of_collection_id = c.id
WHERE o.project_id = $1
  AND COALESCE(o.part_of_collection_id, '') <> ''
  AND c.project_id <> $1
ORDER BY c.id;

-- name: WeaveListCollectionCategoriesForProject :many
-- All categories defined in this project. Used to populate the category
-- filter dropdown on the collection list. The previous
-- USED-only join silently returned 0 rows on projects whose collections
-- weren't yet tagged with default_category_id — making the filter look
-- broken to the customer. All-categories is symmetric with how
-- categories surface elsewhere in the project (sidebar, drafts).
SELECT id, ui_name, canonical_order
FROM weave_categories
WHERE project_id = $1
ORDER BY canonical_order ASC, id ASC;

-- name: WeaveListCollectionScopeClasses :many
-- Returns distinct ontology scope classes (prefix:local_name) used by
-- collections in this project. Drives the scope_class filter dropdown
-- on the collection list.
SELECT DISTINCT ((ontology_scope::jsonb->>'prefix') || ':' || (ontology_scope::jsonb->>'local_name'))::text AS scope_class
FROM weave_collections
WHERE project_id = $1
  AND ontology_scope IS NOT NULL
  AND ontology_scope->>'prefix' IS NOT NULL
  AND ontology_scope->>'local_name' IS NOT NULL
ORDER BY scope_class ASC;

-- name: WeaveGetCollectionNamesByIDs :many
SELECT id, ui_name FROM weave_collections WHERE id = ANY(@ids::text[]);

-- name: WeaveListCollectionOptions :many
-- Lightweight options list for ref pickers — only the columns the UI needs.
-- No COUNT companion query, no path/scope/staging payload, no search arg.
-- Ordered the same as WeaveListCollections so the dropdown matches the
-- canonical list order curators see in the project page.
SELECT id, system_name, ui_name, status, collection_number
FROM weave_collections
WHERE project_id = @project_id::text
  AND deprecated = false
ORDER BY canonical_collection_order ASC, system_name ASC;

-- name: WeaveGetCollectionByIdentifier :one
SELECT * FROM weave_collections
WHERE project_id = @project_id::text
  AND (system_name = @identifier::text OR id = @identifier::text)
LIMIT 1;
