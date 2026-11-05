-- Preserve mode of the legacy rebuild (plan format 2). Read side first.

-- name: WeaveLegacyArchiveDigest :one
-- Plan-time fingerprint of one release's archived sections; the apply re-plans
-- and compares it, so an archive row changed in between refuses the apply.
SELECT encode(sha256(convert_to(weave_legacy_archive_sections(sqlc.arg(project_id)::text,sqlc.arg(version)::text)::text,'UTF8')),'hex')::text AS digest;

-- name: WeaveLegacyArchiveReceipt :one
SELECT weave_legacy_archive_receipt(sqlc.arg(project_id)::text,sqlc.arg(version)::text)::jsonb AS receipt;

-- name: WeaveArchivedFollowingDependencies :many
-- Mirror of WeaveDraftFollowingDependencies over the archived inheritance rows.
SELECT parent_project_id FROM weave_project_inheritance_archive
WHERE project_id = @project_id::text AND version_number = @version::text AND source_mode = 'draft'
ORDER BY parent_project_id;

-- name: WeaveArchivedDependencyReferences :many
-- Mirror of WeaveDraftDependencyReferences (read_dependency_draft/dependency_draft.sql)
-- over the archive sections at one release. Keep the branches in the same order.
-- Example values check their field with a version-free EXISTS: fields are
-- archived only at their owning project's versions, so an adopted foreign
-- field never shares this release's version_number. Overrides are the
-- consumer's own rows and stay joined at the same version.
WITH refs AS (
    SELECT 'project'::text AS entity_type, i.parent_project_id AS entity_id,
           i.parent_project_id AS source_project_id, COALESCE(i.source_version, '')::text AS source_version
    FROM weave_project_inheritance_archive i WHERE i.project_id = @project_id::text AND i.version_number = @version::text
    UNION ALL
    SELECT a.entity_type, a.source_entity_id, a.source_project_id, a.source_version
    FROM weave_adoptions_archive a WHERE a.project_id = @project_id::text AND a.version_number = @version::text
    UNION ALL
    SELECT 'field', o.field_id, '', '' FROM weave_field_overrides_archive o WHERE o.project_id = @project_id::text AND o.version_number = @version::text
    UNION ALL
    SELECT o.entity_type, o.entity_id, '', '' FROM weave_field_overrides_archive o WHERE o.project_id = @project_id::text AND o.version_number = @version::text AND o.entity_type <> ''
    UNION ALL
    SELECT 'category', o.category_id, '', '' FROM weave_field_overrides_archive o WHERE o.project_id = @project_id::text AND o.version_number = @version::text
    UNION ALL
    SELECT 'collection', o.part_of_collection_id, '', '' FROM weave_field_overrides_archive o WHERE o.project_id = @project_id::text AND o.version_number = @version::text
    UNION ALL
    SELECT 'vocabulary_entry', o.set_value_entry_id, '', '' FROM weave_field_overrides_archive o WHERE o.project_id = @project_id::text AND o.version_number = @version::text
    UNION ALL
    SELECT CASE r.ref_type WHEN 'resource_model' THEN 'model' WHEN 'collection_model' THEN 'collection' WHEN 'concept_list' THEN 'concept_list' ELSE r.ref_type END,
           COALESCE(NULLIF(r.target_id, ''), r.semantic_id), '', ''
    FROM weave_override_refs_archive r WHERE r.project_id = @project_id::text AND r.version_number = @version::text
    UNION ALL
    SELECT 'category', c.default_category_id, '', '' FROM weave_collections_archive c WHERE c.project_id = @project_id::text AND c.version_number = @version::text
    UNION ALL
    SELECT 'model', p.model_id, '', '' FROM weave_collection_placements_archive p WHERE p.project_id = @project_id::text AND p.version_number = @version::text
    UNION ALL
    SELECT 'collection', p.collection_id, '', '' FROM weave_collection_placements_archive p WHERE p.project_id = @project_id::text AND p.version_number = @version::text
    UNION ALL
    SELECT 'category', p.category_id, '', '' FROM weave_collection_placements_archive p WHERE p.project_id = @project_id::text AND p.version_number = @version::text
    UNION ALL
    SELECT e.entity_type, e.entity_id, '', '' FROM weave_examples_archive e WHERE e.project_id = @project_id::text AND e.version_number = @version::text
    UNION ALL
    SELECT 'override', o.id::text, '', '' FROM weave_example_values_archive v
        JOIN weave_examples_archive e ON e.id = v.example_id AND e.version_number = v.version_number
        JOIN weave_field_overrides_archive o ON o.id = v.override_id AND o.version_number = v.version_number
        WHERE e.project_id = @project_id::text AND e.version_number = @version::text
          AND EXISTS (SELECT 1 FROM weave_fields_archive f WHERE f.id = v.field_id)
    UNION ALL
    SELECT 'field', v.field_id, '', '' FROM weave_example_values_archive v
        JOIN weave_examples_archive e ON e.id = v.example_id AND e.version_number = v.version_number
        JOIN weave_field_overrides_archive o ON o.id = v.override_id AND o.version_number = v.version_number
        WHERE e.project_id = @project_id::text AND e.version_number = @version::text
          AND EXISTS (SELECT 1 FROM weave_fields_archive f WHERE f.id = v.field_id)
    UNION ALL
    SELECT 'collection', v.part_of_collection_id, '', '' FROM weave_example_values_archive v
        JOIN weave_examples_archive e ON e.id = v.example_id AND e.version_number = v.version_number
        JOIN weave_field_overrides_archive o ON o.id = v.override_id AND o.version_number = v.version_number
        WHERE e.project_id = @project_id::text AND e.version_number = @version::text
          AND EXISTS (SELECT 1 FROM weave_fields_archive f WHERE f.id = v.field_id)
    UNION ALL
    SELECT 'example', v.linked_example_id, '', '' FROM weave_example_values_archive v
        JOIN weave_examples_archive e ON e.id = v.example_id AND e.version_number = v.version_number
        WHERE e.project_id = @project_id::text AND e.version_number = @version::text
    UNION ALL
    SELECT 'vocabulary', c.vocabulary_id, '', '' FROM weave_concept_lists_archive c WHERE c.project_id = @project_id::text AND c.version_number = @version::text
    UNION ALL
    SELECT 'vocabulary_entry', c.list_type, '', '' FROM weave_concept_lists_archive c WHERE c.project_id = @project_id::text AND c.version_number = @version::text
    UNION ALL
    SELECT 'vocabulary_entry', e.vocabulary_entry_id, '', '' FROM weave_concept_list_entries_archive e
        JOIN weave_concept_lists_archive c ON c.id = e.concept_list_id AND c.version_number = e.version_number
        WHERE c.project_id = @project_id::text AND c.version_number = @version::text
    UNION ALL
    SELECT 'vocabulary_entry', b.concept_id, '', '' FROM weave_concept_broader_archive b
        JOIN weave_concept_lists_archive c ON c.id = b.scheme_id AND c.version_number = b.version_number
        WHERE c.project_id = @project_id::text AND c.version_number = @version::text
    UNION ALL
    SELECT 'vocabulary_entry', b.broader_id, '', '' FROM weave_concept_broader_archive b
        JOIN weave_concept_lists_archive c ON c.id = b.scheme_id AND c.version_number = b.version_number
        WHERE c.project_id = @project_id::text AND c.version_number = @version::text
)
SELECT DISTINCT entity_type::text, entity_id::text, source_project_id::text, source_version::text
FROM refs WHERE entity_id IS NOT NULL AND btrim(entity_id) <> ''
ORDER BY source_project_id, source_version DESC, entity_type, entity_id;

-- name: WeaveArchivedDependencyEntityOwner :one
-- Mirror of WeaveDraftDependencyEntityOwner over archive rows. An id's owning
-- project never changes between versions, so any archived version answers.
WITH entities AS (
    SELECT 'project'::text AS entity_type, id, id AS project_id FROM weave_projects_archive
    UNION ALL SELECT 'field', id, project_id FROM weave_fields_archive
    UNION ALL SELECT 'model', id, project_id FROM weave_models_archive
    UNION ALL SELECT 'collection', id, project_id FROM weave_collections_archive
    UNION ALL SELECT 'category', id, project_id FROM weave_categories_archive
    UNION ALL SELECT 'concept_list', id, project_id FROM weave_concept_lists_archive
    UNION ALL SELECT 'example', id, project_id FROM weave_examples_archive
    UNION ALL SELECT 'override', id::text, project_id FROM weave_field_overrides_archive
    UNION ALL SELECT 'vocabulary', id, project_id FROM weave_vocabularies_archive
    UNION ALL SELECT 'vocabulary_entry', e.id, v.project_id FROM weave_vocabulary_entries_archive e
        JOIN weave_vocabularies_archive v ON v.id = e.vocabulary_id AND v.version_number = e.version_number
)
SELECT COALESCE(project_id, '')::text AS project_id FROM entities
WHERE entity_type = @entity_type::text AND id = @entity_id::text
LIMIT 1;

-- name: WeavePinArchivedAdoptions :exec
-- Archive twin of WeaveSnapshotPinnedAdoptions: stamp the captured source pin
-- onto archived adoption receipts that were unpinned when the release was cut.
-- Runs while archive ownership is 0, so the content guard allows the update.
UPDATE weave_adoptions_archive a SET source_version = d.source_version
FROM weave_release_project_dependencies d
WHERE a.project_id = @project_id::text AND a.version_number = @version::text
  AND d.project_id = a.project_id AND d.version_number = a.version_number
  AND d.source_project_id = a.source_project_id AND a.source_version = '';

-- name: WeavePreservedArchiveState :one
SELECT count(*)::bigint AS project_rows FROM weave_projects_archive
WHERE id = sqlc.arg(project_id)::text AND version_number = sqlc.arg(version)::text;

-- name: WeavePreserveReleaseRow :exec
-- Re-insert a release with its reviewed original catalog metadata. The
-- ordinary insert (insertReleaseInTx) stamps now() and the rebuild actor.
INSERT INTO weave_releases (project_id, version, title, description, created_at, created_by_id)
VALUES (sqlc.arg(project_id)::text, sqlc.arg(version)::text, sqlc.arg(title)::text, sqlc.arg(description)::text,
        sqlc.arg(created_at)::timestamptz, sqlc.arg(created_by_id)::text);

-- name: WeaveInsertReleaseChangeSet :exec
INSERT INTO weave_change_set (project_id, actor_id, actor_name, actor_email, commit_message, started_at, closed_at, kind, release_version)
VALUES (sqlc.arg(project_id)::text, sqlc.arg(actor_id)::text, 'pletka-system', 'system@pletka.local',
        sqlc.arg(commit_message)::text, now(), now(), 'release', sqlc.arg(version)::text);

-- name: WeaveInsertPreservedReleaseImport :exec
-- The migration 041 ledger row for a preserved release. Header values come from
-- the release row inserted in this transaction; the receipt is computed from
-- the archive rows and must equal what finalization computes.
INSERT INTO weave_release_imports (project_id, version_number, document)
SELECT r.project_id, r.version, jsonb_build_object(
    'header', jsonb_build_object('project_id', r.project_id, 'version', r.version,
        'created_at', to_char(r.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
        'created_by_id', r.created_by_id, 'title', r.title, 'description', r.description),
    'receipt', weave_legacy_archive_receipt(r.project_id, r.version),
    'source', 'preserved-legacy-archive',
    'rebuild_operation', sqlc.arg(operation)::text)
FROM weave_releases r
WHERE r.project_id = sqlc.arg(project_id)::text AND r.version = sqlc.arg(version)::text;
