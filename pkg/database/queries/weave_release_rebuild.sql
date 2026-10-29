-- name: WeaveTryRuntimeLease :one
SELECT pg_try_advisory_lock_shared(721034001)::boolean;
-- name: WeaveReleaseRuntimeLease :exec
SELECT pg_advisory_unlock_shared(721034001);
-- name: WeaveTryRebuildLease :one
SELECT pg_try_advisory_lock(721034001)::boolean;
-- name: WeaveReleaseRebuildLease :exec
SELECT pg_advisory_unlock(721034001);
-- name: WeaveRebuildPending :one
SELECT EXISTS(SELECT 1 FROM weave_release_rebuild_operations WHERE completed_at IS NULL);
-- name: WeaveLockRebuildInputs :exec
-- Utility LOCK does not acquire an MVCC snapshot; validation starts only after
-- any already-running writer commits. Kept exhaustive by the capture ledger test.
LOCK TABLE weave_actors,
    weave_adoptions,
    weave_adoptions_archive,
    weave_api_keys,
    weave_auth,
    weave_categories,
    weave_categories_archive,
    weave_change_log,
    weave_change_log_archive,
    weave_change_set,
    weave_change_set_archive,
    weave_collection_placements,
    weave_collection_placements_archive,
    weave_collections,
    weave_collections_archive,
    weave_concept_broader,
    weave_concept_broader_archive,
    weave_concept_list_entries,
    weave_concept_list_entries_archive,
    weave_concept_lists,
    weave_concept_lists_archive,
    weave_entity_counters,
    weave_entity_forks,
    weave_entity_forks_archive,
    weave_error_events,
    weave_example_values,
    weave_example_values_archive,
    weave_examples,
    weave_examples_archive,
    weave_field_ontology_refs,
    weave_field_overrides,
    weave_field_overrides_archive,
    weave_fields,
    weave_fields_archive,
    weave_import_staging,
    weave_memberships,
    weave_models,
    weave_models_archive,
    weave_namespace_bindings,
    weave_namespace_bindings_archive,
    weave_ontologies,
    weave_ontology_active_versions,
    weave_legacy_ontology_snapshots,
    weave_ontology_body_seals,
    weave_ontology_classes,
    weave_ontology_draft_publications,
    weave_ontology_drafts,
    weave_ontology_families,
    weave_ontology_properties,
    weave_ontology_relations,
    weave_ontology_release_dependencies,
    weave_ontology_releases,
    weave_ontology_version_companions,
    weave_ontology_versions,
    weave_override_refs,
    weave_override_refs_archive,
    weave_path_elements,
    weave_project_actors,
    weave_project_actors_archive,
    weave_project_attributions,
    weave_project_attributions_archive,
    weave_project_inheritance,
    weave_project_inheritance_archive,
    weave_project_integrations,
    weave_project_ontology_versions,
    weave_project_ontology_versions_archive,
    weave_projects,
    weave_projects_archive,
    weave_release_content_fences,
    weave_release_dependency_entities,
    weave_release_descriptors,
    weave_release_metadata_fences,
    weave_release_namespace_bindings_archive,
    weave_release_ontology_dependencies,
    weave_release_ontology_references,
    weave_release_project_dependencies,
    weave_release_rebuild_operations,
    weave_releases,
    weave_vocabularies,
    weave_vocabularies_archive,
    weave_vocabulary_entries,
    weave_vocabulary_entries_archive IN SHARE ROW EXCLUSIVE MODE;
-- name: WeaveRebuildFingerprint :one
SELECT weave_release_rebuild_fingerprint(sqlc.arg(project_ids)::text[])::text;
-- name: WeaveRebuildCatalog :many
SELECT r.project_id,r.version,r.title,r.description,r.created_by_id,r.created_at,r.content_capture_version,r.archived_at,
       COALESCE(p.parent_project_id,'')::text AS legacy_parent,
       EXISTS(SELECT 1 FROM weave_legacy_ontology_snapshots s WHERE s.project_id=r.project_id AND s.version_number=r.version)::boolean AS ontology_snapshot_captured
FROM weave_releases r JOIN weave_projects p ON p.id=r.project_id ORDER BY r.project_id,r.version;
-- name: WeaveRebuildRetainedSources :many
SELECT project_id,source_project_id FROM weave_release_project_dependencies ORDER BY project_id,source_project_id;
-- name: WeaveRebuildInheritance :many
SELECT project_id,parent_project_id,source_mode,COALESCE(source_version,'')::text AS source_version FROM weave_project_inheritance WHERE project_id=ANY(sqlc.arg(project_ids)::text[]) ORDER BY project_id,parent_project_id;
-- name: WeaveRebuildPinInheritance :exec
UPDATE weave_project_inheritance SET source_mode='release',source_version=sqlc.arg(version)::text
WHERE project_id=sqlc.arg(project_id)::text AND parent_project_id=sqlc.arg(parent_project_id)::text;
-- name: WeaveRebuildDeleteRelease :exec
DELETE FROM weave_releases WHERE project_id=sqlc.arg(project_id)::text AND version=sqlc.arg(version)::text AND content_capture_version=0;
-- name: WeaveRebuildClearArchive :exec
SELECT weave_clear_release_for_rebuild(sqlc.arg(project_id)::text,sqlc.arg(version)::text);
-- name: WeaveRebuildSupersedeOutbox :exec
UPDATE weave_change_set SET processed_at=now(),materialized_outcome='release_rebuilt',materialized_error=NULL
WHERE project_id=ANY(sqlc.arg(project_ids)::text[]) AND kind='release' AND processed_at IS NULL;
-- name: WeaveRebuildCapturedResult :one
SELECT r.content_capture_receipt,c.id AS change_set_id FROM weave_releases r
JOIN weave_change_set c ON c.project_id=r.project_id AND c.release_version=r.version AND c.kind='release' AND c.processed_at IS NULL
WHERE r.project_id=sqlc.arg(project_id)::text AND r.version=sqlc.arg(version)::text AND r.content_capture_version IN (1,2);
-- name: WeaveInsertRebuildOperation :exec
INSERT INTO weave_release_rebuild_operations(id,document,result) VALUES (sqlc.arg(id)::text,sqlc.arg(document)::text,sqlc.arg(result)::jsonb);
-- name: WeaveGetRebuildOperation :one
SELECT * FROM weave_release_rebuild_operations WHERE id=sqlc.arg(id)::text;
-- name: WeaveCompleteRebuildOutbox :exec
UPDATE weave_change_set SET processed_at=now(),materialized_outcome='committed',materialized_error=NULL,git_commit_sha=sqlc.arg(commit_sha)::text
WHERE id=sqlc.arg(id)::bigint AND kind='release';
-- name: WeaveCompleteRebuildOperation :exec
UPDATE weave_release_rebuild_operations SET completed_at=COALESCE(completed_at,now()) WHERE id=sqlc.arg(id)::text;
-- name: WeaveVerifyRebuiltRelease :one
SELECT EXISTS(
 SELECT 1 FROM weave_releases r JOIN weave_change_set c ON c.project_id=r.project_id AND c.release_version=r.version
 WHERE r.project_id=sqlc.arg(project_id)::text AND r.version=sqlc.arg(version)::text
 AND r.content_capture_version IN (1,2) AND r.content_capture_receipt=sqlc.arg(receipt)::jsonb
 AND c.id=sqlc.arg(change_set_id)::bigint AND c.kind='release' AND c.processed_at IS NULL
);
