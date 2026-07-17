-- name: WeaveSearchFields :many
-- Searches fields by free text and/or path element.
-- Scope: project-only when @scope = 'project', project + parent chain when @scope = 'inherited'.
-- NOTE: Adoption count uses LEFT JOIN on weave_field_overrides. Adds ~80ms for 617 fields.
-- If this becomes a bottleneck, denormalize adoption_count into weave_fields.
WITH RECURSIVE project_chain AS (
    SELECT id, ARRAY[id] AS path
    FROM weave_projects
    WHERE id = @project_id::text
    UNION ALL
    SELECT next_parent.parent_id, pc.path || next_parent.parent_id
    FROM project_chain pc
    JOIN LATERAL (
        SELECT wpi.parent_project_id AS parent_id
        FROM weave_project_inheritance wpi
        WHERE wpi.project_id = pc.id
        UNION ALL
        SELECT p.parent_project_id AS parent_id
        FROM weave_projects p
        WHERE p.id = pc.id
          AND p.parent_project_id IS NOT NULL
          AND NOT EXISTS (
              SELECT 1 FROM weave_project_inheritance wpi
              WHERE wpi.project_id = pc.id
          )
    ) next_parent ON TRUE
    WHERE @scope::text = 'inherited'
      AND next_parent.parent_id IS NOT NULL
      AND next_parent.parent_id <> ''
      AND NOT (next_parent.parent_id = ANY(pc.path))
),
matching_fields AS (
    -- category_id reads from the field's base override (entity_type='').
    -- Migration 027 dropped weave_fields.category_id; the base override
    -- is now the canonical home.
    SELECT f.id, f.semantic_id, f.system_name, f.ui_name, f.description,
           f.ontology_scope, f.ontology_path, f.path_elements, f.expected_value_type,
           f.project_id,
           COALESCE(NULLIF(bo.category_id, ''), '') AS category_id,
           (f.project_id = @project_id::text) AS is_local
    FROM weave_fields f
    LEFT JOIN weave_field_overrides bo
      ON bo.field_id = f.id AND bo.entity_type = ''
    WHERE f.project_id IN (SELECT id FROM project_chain)
      AND f.status IN ('draft', 'published')
      -- Free text search (skip if empty)
      AND (@search::text = '' OR (
           EXISTS (SELECT 1 FROM jsonb_each_text(f.ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
           OR EXISTS (SELECT 1 FROM jsonb_each_text(f.description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
           OR f.system_name ILIKE '%' || @search::text || '%'
           OR COALESCE(f.semantic_id, '') ILIKE '%' || @search::text || '%'
           OR COALESCE(f.ontology_path, '') ILIKE '%' || @search::text || '%'
      ))
      -- Path element filter (skip if empty)
      -- Path element filter: uses the indexed weave_path_elements table.
      -- The handler pre-parses the input into @path_prefix and @path_local_name.
      -- When prefix is set, both must match. When only local_name, it matches alone.
      AND (@path_local_name::text = '' OR f.id IN (
           SELECT pe.field_id FROM weave_path_elements pe
           WHERE starts_with(pe.local_name, @path_local_name::text)
             AND (@path_prefix::text = '' OR pe.prefix = @path_prefix::text)
      ))
      -- Category filter (skip if empty); reads category from base override.
      AND (@category::text = '' OR bo.category_id = @category::text)
      -- Expected value type filter (skip if empty)
      AND (@expected_value_type::text = '' OR f.expected_value_type = @expected_value_type::text)
      -- Ontology scope class filter: matches primary scope OR any additional_types entry
      AND (
        (@ontology_class::text = '' AND @ontology_prefix::text = '')
        OR (
          (@ontology_class::text = '' OR starts_with(f.ontology_scope->>'local_name', @ontology_class::text))
          AND (@ontology_prefix::text = '' OR f.ontology_scope->>'prefix' = @ontology_prefix::text)
        )
        OR EXISTS (
          SELECT 1 FROM jsonb_array_elements(COALESCE(f.ontology_scope->'additional_types', '[]'::jsonb)) at
          WHERE (@ontology_class::text = '' OR starts_with(at->>'local_name', @ontology_class::text))
            AND (@ontology_prefix::text = '' OR at->>'prefix' = @ontology_prefix::text)
        )
      )
      -- Pre-computed path field IDs (from stored function, empty = no filter)
      AND (cardinality(@path_field_ids::text[]) = 0 OR f.id = ANY(@path_field_ids::text[]))
),
with_adoption AS (
    SELECT mf.*,
           COALESCE(COUNT(DISTINCT fo.entity_id), 0) AS adoption_count
    FROM matching_fields mf
    LEFT JOIN weave_field_overrides fo
      ON fo.field_id = mf.id AND fo.entity_type IN ('model', 'collection')
    GROUP BY mf.id, mf.semantic_id, mf.system_name, mf.ui_name, mf.description,
             mf.ontology_scope, mf.ontology_path, mf.path_elements, mf.expected_value_type,
             mf.project_id, mf.category_id, mf.is_local
)
SELECT * FROM with_adoption
ORDER BY
    CASE WHEN @sort_by::text = 'adoption' THEN adoption_count END DESC NULLS LAST,
    CASE WHEN @sort_by::text = 'name' THEN ui_name->>'en' END ASC NULLS LAST,
    CASE WHEN @sort_by::text = '' OR @sort_by::text = 'relevance' THEN is_local::int END DESC,
    CASE WHEN @sort_by::text = '' OR @sort_by::text = 'relevance' THEN adoption_count END DESC NULLS LAST,
    semantic_id ASC
LIMIT @result_limit::integer OFFSET @result_offset::integer;

-- name: WeaveCountSearchFields :one
-- Count matching fields for total_count in search response.
WITH RECURSIVE project_chain AS (
    SELECT id, ARRAY[id] AS path
    FROM weave_projects
    WHERE id = @project_id::text
    UNION ALL
    SELECT next_parent.parent_id, pc.path || next_parent.parent_id
    FROM project_chain pc
    JOIN LATERAL (
        SELECT wpi.parent_project_id AS parent_id
        FROM weave_project_inheritance wpi
        WHERE wpi.project_id = pc.id
        UNION ALL
        SELECT p.parent_project_id AS parent_id
        FROM weave_projects p
        WHERE p.id = pc.id
          AND p.parent_project_id IS NOT NULL
          AND NOT EXISTS (
              SELECT 1 FROM weave_project_inheritance wpi
              WHERE wpi.project_id = pc.id
          )
    ) next_parent ON TRUE
    WHERE @scope::text = 'inherited'
      AND next_parent.parent_id IS NOT NULL
      AND next_parent.parent_id <> ''
      AND NOT (next_parent.parent_id = ANY(pc.path))
)
SELECT COUNT(*) FROM weave_fields f
LEFT JOIN weave_field_overrides bo
  ON bo.field_id = f.id AND bo.entity_type = ''
WHERE f.project_id IN (SELECT id FROM project_chain)
  AND f.status IN ('draft', 'published')
  AND (@search::text = '' OR (
       EXISTS (SELECT 1 FROM jsonb_each_text(f.ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR EXISTS (SELECT 1 FROM jsonb_each_text(f.description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR f.system_name ILIKE '%' || @search::text || '%'
       OR COALESCE(f.semantic_id, '') ILIKE '%' || @search::text || '%'
       OR COALESCE(f.ontology_path, '') ILIKE '%' || @search::text || '%'
  ))
  AND (@path_local_name::text = '' OR f.id IN (
       SELECT pe.field_id FROM weave_path_elements pe
       WHERE starts_with(pe.local_name, @path_local_name::text)
         AND (@path_prefix::text = '' OR pe.prefix = @path_prefix::text)
  ))
  -- Category filter reads from base override (post-migration 027).
  AND (@category::text = '' OR bo.category_id = @category::text)
  AND (@expected_value_type::text = '' OR f.expected_value_type = @expected_value_type::text)
  AND (
    (@ontology_class::text = '' AND @ontology_prefix::text = '')
    OR (
      (@ontology_class::text = '' OR starts_with(f.ontology_scope->>'local_name', @ontology_class::text))
      AND (@ontology_prefix::text = '' OR f.ontology_scope->>'prefix' = @ontology_prefix::text)
    )
    OR EXISTS (
      SELECT 1 FROM jsonb_array_elements(COALESCE(f.ontology_scope->'additional_types', '[]'::jsonb)) at
      WHERE (@ontology_class::text = '' OR starts_with(at->>'local_name', @ontology_class::text))
        AND (@ontology_prefix::text = '' OR at->>'prefix' = @ontology_prefix::text)
    )
  )
  AND (cardinality(@path_field_ids::text[]) = 0 OR f.id = ANY(@path_field_ids::text[]));

-- name: WeaveSearchCollections :many
-- Searches collections by free text and/or path element (collections containing matching fields).
WITH RECURSIVE project_chain AS (
    SELECT id, ARRAY[id] AS path
    FROM weave_projects
    WHERE id = @project_id::text
    UNION ALL
    SELECT next_parent.parent_id, pc.path || next_parent.parent_id
    FROM project_chain pc
    JOIN LATERAL (
        SELECT wpi.parent_project_id AS parent_id
        FROM weave_project_inheritance wpi
        WHERE wpi.project_id = pc.id
        UNION ALL
        SELECT p.parent_project_id AS parent_id
        FROM weave_projects p
        WHERE p.id = pc.id
          AND p.parent_project_id IS NOT NULL
          AND NOT EXISTS (
              SELECT 1 FROM weave_project_inheritance wpi
              WHERE wpi.project_id = pc.id
          )
    ) next_parent ON TRUE
    WHERE @scope::text = 'inherited'
      AND next_parent.parent_id IS NOT NULL
      AND next_parent.parent_id <> ''
      AND NOT (next_parent.parent_id = ANY(pc.path))
),
matching_collections AS (
    SELECT c.id, c.system_name, c.ui_name, c.description,
           c.ontology_scope, c.project_id,
           (c.project_id = @project_id::text) AS is_local
    FROM weave_collections c
    WHERE c.project_id IN (SELECT id FROM project_chain)
      AND c.status IN ('draft', 'published')
      -- Free text search
      AND (@search::text = '' OR (
           EXISTS (SELECT 1 FROM jsonb_each_text(c.ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
           OR EXISTS (SELECT 1 FROM jsonb_each_text(c.description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
           OR c.system_name ILIKE '%' || @search::text || '%'
           OR c.id ILIKE '%' || @search::text || '%'
      ))
      -- Path element filter: find collections that contain fields with this path element
      AND (@path_local_name::text = '' OR c.id IN (
           SELECT DISTINCT fo.entity_id
           FROM weave_path_elements pe
           JOIN weave_field_overrides fo ON fo.field_id = pe.field_id AND fo.entity_type = 'collection'
           WHERE starts_with(pe.local_name, @path_local_name::text)
             AND (@path_prefix::text = '' OR pe.prefix = @path_prefix::text)
      ))
      -- Ontology scope class filter for collections: matches primary scope OR any additional_types entry
      AND (
        (@ontology_class::text = '' AND @ontology_prefix::text = '')
        OR (
          (@ontology_class::text = '' OR starts_with(c.ontology_scope->>'local_name', @ontology_class::text))
          AND (@ontology_prefix::text = '' OR c.ontology_scope->>'prefix' = @ontology_prefix::text)
        )
        OR EXISTS (
          SELECT 1 FROM jsonb_array_elements(COALESCE(c.ontology_scope->'additional_types', '[]'::jsonb)) at
          WHERE (@ontology_class::text = '' OR starts_with(at->>'local_name', @ontology_class::text))
            AND (@ontology_prefix::text = '' OR at->>'prefix' = @ontology_prefix::text)
        )
      )
      -- Pre-computed path field IDs (from stored function, empty = no filter)
      AND (cardinality(@path_field_ids::text[]) = 0 OR c.id IN (
           SELECT DISTINCT fo.entity_id
           FROM weave_field_overrides fo
           WHERE fo.entity_type = 'collection'
             AND fo.field_id = ANY(@path_field_ids::text[])
      ))
),
with_adoption AS (
    SELECT mc.*,
           -- Adoption = how many distinct models reference fields with part_of_collection_id = this collection
           COALESCE((
               SELECT COUNT(DISTINCT fo.entity_id)
               FROM weave_field_overrides fo
               WHERE fo.part_of_collection_id = mc.id
                 AND fo.entity_type = 'model'
           ), 0) AS adoption_count
    FROM matching_collections mc
)
SELECT * FROM with_adoption
ORDER BY
    CASE WHEN @sort_by::text = 'adoption' THEN adoption_count END DESC NULLS LAST,
    CASE WHEN @sort_by::text = 'name' THEN ui_name->>'en' END ASC NULLS LAST,
    CASE WHEN @sort_by::text = '' OR @sort_by::text = 'relevance' THEN is_local::int END DESC,
    CASE WHEN @sort_by::text = '' OR @sort_by::text = 'relevance' THEN adoption_count END DESC NULLS LAST,
    id ASC
LIMIT @result_limit::integer OFFSET @result_offset::integer;

-- name: WeaveCountSearchCollections :one
-- Count matching collections for total_count in search response.
WITH RECURSIVE project_chain AS (
    SELECT id, ARRAY[id] AS path
    FROM weave_projects
    WHERE id = @project_id::text
    UNION ALL
    SELECT next_parent.parent_id, pc.path || next_parent.parent_id
    FROM project_chain pc
    JOIN LATERAL (
        SELECT wpi.parent_project_id AS parent_id
        FROM weave_project_inheritance wpi
        WHERE wpi.project_id = pc.id
        UNION ALL
        SELECT p.parent_project_id AS parent_id
        FROM weave_projects p
        WHERE p.id = pc.id
          AND p.parent_project_id IS NOT NULL
          AND NOT EXISTS (
              SELECT 1 FROM weave_project_inheritance wpi
              WHERE wpi.project_id = pc.id
          )
    ) next_parent ON TRUE
    WHERE @scope::text = 'inherited'
      AND next_parent.parent_id IS NOT NULL
      AND next_parent.parent_id <> ''
      AND NOT (next_parent.parent_id = ANY(pc.path))
)
SELECT COUNT(*) FROM weave_collections c
WHERE c.project_id IN (SELECT id FROM project_chain)
  AND c.status IN ('draft', 'published')
  AND (@search::text = '' OR (
       EXISTS (SELECT 1 FROM jsonb_each_text(c.ui_name) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR EXISTS (SELECT 1 FROM jsonb_each_text(c.description) jt WHERE jt.value ILIKE '%' || @search::text || '%')
       OR c.system_name ILIKE '%' || @search::text || '%'
       OR c.id ILIKE '%' || @search::text || '%'
  ))
  AND (@path_local_name::text = '' OR c.id IN (
       SELECT DISTINCT fo.entity_id
       FROM weave_path_elements pe
       JOIN weave_field_overrides fo ON fo.field_id = pe.field_id AND fo.entity_type = 'collection'
       WHERE starts_with(pe.local_name, @path_local_name::text)
         AND (@path_prefix::text = '' OR pe.prefix = @path_prefix::text)
  ))
  AND (
    (@ontology_class::text = '' AND @ontology_prefix::text = '')
    OR (
      (@ontology_class::text = '' OR starts_with(c.ontology_scope->>'local_name', @ontology_class::text))
      AND (@ontology_prefix::text = '' OR c.ontology_scope->>'prefix' = @ontology_prefix::text)
    )
    OR EXISTS (
      SELECT 1 FROM jsonb_array_elements(COALESCE(c.ontology_scope->'additional_types', '[]'::jsonb)) at
      WHERE (@ontology_class::text = '' OR starts_with(at->>'local_name', @ontology_class::text))
        AND (@ontology_prefix::text = '' OR at->>'prefix' = @ontology_prefix::text)
    )
  )
  AND (cardinality(@path_field_ids::text[]) = 0 OR c.id IN (
       SELECT DISTINCT fo.entity_id
       FROM weave_field_overrides fo
       WHERE fo.entity_type = 'collection'
         AND fo.field_id = ANY(@path_field_ids::text[])
  ));

-- name: WeaveGetLinkedFieldIDs :many
-- Returns field IDs already linked to a target model/collection (for is_linked marking).
SELECT DISTINCT field_id FROM weave_field_overrides
WHERE entity_type = @entity_type::text AND entity_id = @entity_id::text;
