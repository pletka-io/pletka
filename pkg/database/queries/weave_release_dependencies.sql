-- name: WeaveAuditReleaseProjectDependencies :many
-- Report history, never reconstruct it from current drafts.
SELECT project_id, version, created_at, archive_ownership_version
FROM weave_releases WHERE project_dependencies_version <> 1
ORDER BY project_id, created_at, version;

-- name: WeaveInsertReleaseProjectDependency :exec
INSERT INTO weave_release_project_dependencies (project_id, version_number, source_project_id, source_version)
VALUES (@project_id::text, @version::text, @source_project_id::text, @source_version::text);

-- name: WeaveInsertReleaseDependencyEntity :exec
INSERT INTO weave_release_dependency_entities (project_id, version_number, entity_type, entity_id, source_project_id, source_version)
VALUES (@project_id::text, @version::text, @entity_type::text, @entity_id::text, @source_project_id::text, @source_version::text);

-- name: WeaveMarkReleaseProjectDependencies :exec
UPDATE weave_releases SET project_dependencies_version = 1
WHERE project_id = @project_id::text AND version = @version::text;

-- name: WeaveSnapshotPinnedAdoptions :exec
-- Normalize matching explicit/unpinned receipts onto their captured source pin.
-- The oldest receipt carries provenance if normalization merges duplicates.
INSERT INTO weave_adoptions_archive (
    project_id, context_entity_type, context_entity_id, entity_type,
    source_project_id, source_entity_id, source_version, adopted_at, created_by_id, version_number
)
SELECT DISTINCT ON (a.context_entity_type, a.context_entity_id, a.entity_type, a.source_project_id, a.source_entity_id)
    a.project_id, a.context_entity_type, a.context_entity_id, a.entity_type,
    a.source_project_id, a.source_entity_id, COALESCE(d.source_version, a.source_version),
    a.adopted_at, a.created_by_id, @version::text
FROM weave_adoptions a
LEFT JOIN weave_release_project_dependencies d ON d.project_id = a.project_id
    AND d.version_number = @version::text AND d.source_project_id = a.source_project_id
WHERE a.project_id = @project_id::text
ORDER BY a.context_entity_type, a.context_entity_id, a.entity_type, a.source_project_id, a.source_entity_id, a.adopted_at, a.id;
