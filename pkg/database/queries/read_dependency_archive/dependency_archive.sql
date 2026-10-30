-- name: WeaveReleaseDependencyState :one
SELECT archive_ownership_version, project_dependencies_version
FROM weave_releases WHERE project_id = @project_id::text AND version = @version::text;

-- name: WeaveListReleaseProjectDependencies :many
SELECT * FROM weave_release_project_dependencies
WHERE project_id = @project_id::text AND version_number = @version::text
ORDER BY source_project_id;

-- name: WeaveGetReleaseDependencyEntity :one
SELECT * FROM weave_release_dependency_entities
WHERE project_id = @project_id::text AND version_number = @version::text
  AND entity_type = @entity_type::text AND entity_id = @entity_id::text;

-- name: WeaveReleaseContainsDependencyEntity :one
WITH entities AS (
    SELECT 'project'::text AS entity_type, id, id AS project_id, version_number FROM weave_projects_archive
    UNION ALL SELECT 'field', id, project_id, version_number FROM weave_fields_archive
    UNION ALL SELECT 'model', id, project_id, version_number FROM weave_models_archive
    UNION ALL SELECT 'collection', id, project_id, version_number FROM weave_collections_archive
    UNION ALL SELECT 'category', id, project_id, version_number FROM weave_categories_archive
    UNION ALL SELECT 'concept_list', id, project_id, version_number FROM weave_concept_lists_archive
    UNION ALL SELECT 'example', id, project_id, version_number FROM weave_examples_archive
    UNION ALL SELECT 'override', id::text, project_id, version_number FROM weave_field_overrides_archive
    UNION ALL SELECT 'vocabulary', id, project_id, version_number FROM weave_vocabularies_archive
    UNION ALL SELECT 'vocabulary_entry', e.id, v.project_id, e.version_number
        FROM weave_vocabulary_entries_archive e JOIN weave_vocabularies_archive v ON v.id = e.vocabulary_id AND v.version_number = e.version_number
)
SELECT EXISTS (SELECT 1 FROM entities WHERE entity_type = @entity_type::text AND id = @entity_id::text
    AND project_id = @project_id::text AND version_number = @version::text);

-- name: WeaveListReleaseDependencyEntities :many
SELECT * FROM weave_release_dependency_entities
WHERE project_id = @project_id::text AND version_number = @version::text
ORDER BY entity_type, entity_id;
