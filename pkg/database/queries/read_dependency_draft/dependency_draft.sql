-- name: WeaveLatestDependencyRelease :one
-- Match request policy's numeric-core ordering. Never silently select an older
-- verified release when the latest exists but lacks captured dependencies.
SELECT version FROM weave_releases
WHERE project_id = @project_id::text AND archived_at IS NULL
ORDER BY string_to_array(split_part(split_part(version, '-', 1), '+', 1), '.')::numeric[] DESC,
         created_at DESC, version DESC
LIMIT 1;

-- name: WeaveDraftDependencyReferences :many
WITH refs AS (
    SELECT 'project'::text AS entity_type, i.parent_project_id AS entity_id,
           i.parent_project_id AS source_project_id, COALESCE(i.source_version, '')::text AS source_version
    FROM weave_project_inheritance i WHERE i.project_id = @project_id::text
    UNION ALL
    SELECT a.entity_type, a.source_entity_id, a.source_project_id, a.source_version
    FROM weave_adoptions a WHERE a.project_id = @project_id::text
    UNION ALL
    SELECT 'field', o.field_id, '', '' FROM weave_field_overrides o WHERE o.project_id = @project_id::text
    UNION ALL
    SELECT o.entity_type, o.entity_id, '', '' FROM weave_field_overrides o WHERE o.project_id = @project_id::text AND o.entity_type <> ''
    UNION ALL
    SELECT 'category', o.category_id, '', '' FROM weave_field_overrides o WHERE o.project_id = @project_id::text
    UNION ALL
    SELECT 'collection', o.part_of_collection_id, '', '' FROM weave_field_overrides o WHERE o.project_id = @project_id::text
    UNION ALL
    SELECT 'vocabulary_entry', o.set_value_entry_id, '', '' FROM weave_field_overrides o WHERE o.project_id = @project_id::text
    UNION ALL
    SELECT CASE r.ref_type WHEN 'resource_model' THEN 'model' WHEN 'collection_model' THEN 'collection' WHEN 'concept_list' THEN 'concept_list' ELSE r.ref_type END,
           COALESCE(NULLIF(r.target_id, ''), r.semantic_id), '', ''
    FROM weave_override_refs r JOIN weave_field_overrides o ON o.id = r.override_id WHERE o.project_id = @project_id::text
    UNION ALL
    SELECT 'category', c.default_category_id, '', '' FROM weave_collections c WHERE c.project_id = @project_id::text
    UNION ALL
    SELECT 'model', p.model_id, '', '' FROM weave_collection_placements p WHERE p.project_id = @project_id::text
    UNION ALL
    SELECT 'collection', p.collection_id, '', '' FROM weave_collection_placements p WHERE p.project_id = @project_id::text
    UNION ALL
    SELECT 'category', p.category_id, '', '' FROM weave_collection_placements p WHERE p.project_id = @project_id::text
    UNION ALL
    SELECT e.entity_type, e.entity_id, '', '' FROM weave_examples e WHERE e.project_id = @project_id::text
    -- Saved values whose placement/field was deleted are intentionally retained
    -- as stale evidence (migration 010), not dereferenced as content dependencies.
    UNION ALL
    SELECT 'override', o.id::text, '', '' FROM weave_example_values v JOIN weave_examples e ON e.id = v.example_id
        JOIN weave_field_overrides o ON o.id = v.override_id JOIN weave_fields f ON f.id = v.field_id WHERE e.project_id = @project_id::text
    UNION ALL
    SELECT 'field', v.field_id, '', '' FROM weave_example_values v JOIN weave_examples e ON e.id = v.example_id
        JOIN weave_field_overrides o ON o.id = v.override_id JOIN weave_fields f ON f.id = v.field_id WHERE e.project_id = @project_id::text
    UNION ALL
    SELECT 'collection', v.part_of_collection_id, '', '' FROM weave_example_values v JOIN weave_examples e ON e.id = v.example_id
        JOIN weave_field_overrides o ON o.id = v.override_id JOIN weave_fields f ON f.id = v.field_id WHERE e.project_id = @project_id::text
    UNION ALL
    SELECT 'example', v.linked_example_id, '', '' FROM weave_example_values v JOIN weave_examples e ON e.id = v.example_id WHERE e.project_id = @project_id::text
    UNION ALL
    SELECT 'vocabulary', c.vocabulary_id, '', '' FROM weave_concept_lists c WHERE c.project_id = @project_id::text
    UNION ALL
    SELECT 'vocabulary_entry', c.list_type, '', '' FROM weave_concept_lists c WHERE c.project_id = @project_id::text
    UNION ALL
    SELECT 'vocabulary_entry', e.vocabulary_entry_id, '', '' FROM weave_concept_list_entries e JOIN weave_concept_lists c ON c.id = e.concept_list_id WHERE c.project_id = @project_id::text
    UNION ALL
    SELECT 'vocabulary_entry', b.concept_id, '', '' FROM weave_concept_broader b JOIN weave_concept_lists c ON c.id = b.scheme_id WHERE c.project_id = @project_id::text
    UNION ALL
    SELECT 'vocabulary_entry', b.broader_id, '', '' FROM weave_concept_broader b JOIN weave_concept_lists c ON c.id = b.scheme_id WHERE c.project_id = @project_id::text
)
SELECT DISTINCT entity_type::text, entity_id::text, source_project_id::text, source_version::text
FROM refs WHERE entity_id IS NOT NULL AND btrim(entity_id) <> ''
ORDER BY source_project_id, source_version DESC, entity_type, entity_id;

-- name: WeaveDraftDependencyEntityOwner :one
WITH entities AS (
    SELECT 'project'::text AS entity_type, id, id AS project_id FROM weave_projects
    UNION ALL SELECT 'field', id, project_id FROM weave_fields
    UNION ALL SELECT 'model', id, project_id FROM weave_models
    UNION ALL SELECT 'collection', id, project_id FROM weave_collections
    UNION ALL SELECT 'category', id, project_id FROM weave_categories
    UNION ALL SELECT 'concept_list', id, project_id FROM weave_concept_lists
    UNION ALL SELECT 'example', id, project_id FROM weave_examples
    UNION ALL SELECT 'override', id::text, project_id FROM weave_field_overrides
    UNION ALL SELECT 'vocabulary', id, project_id FROM weave_vocabularies
    UNION ALL SELECT 'vocabulary_entry', e.id, v.project_id FROM weave_vocabulary_entries e JOIN weave_vocabularies v ON v.id = e.vocabulary_id
)
SELECT COALESCE(project_id, '')::text AS project_id FROM entities
WHERE entity_type = @entity_type::text AND id = @entity_id::text;


-- name: WeaveDraftFollowingDependencies :many
-- An explicit follow-draft edge must never become an implicit latest release.
SELECT parent_project_id FROM weave_project_inheritance
WHERE project_id = @project_id::text AND source_mode = 'draft'
ORDER BY parent_project_id;
