-- name: WeaveReleaseHasArchiveOwnership :one
SELECT EXISTS (
    SELECT 1 FROM weave_releases
    WHERE project_id = sqlc.arg(project_id)::text AND version = sqlc.arg(version)::text AND archive_ownership_version = 1
);

-- name: WeaveMarkReleaseArchiveOwnership :exec
UPDATE weave_releases SET archive_ownership_version = 1
WHERE project_id = sqlc.arg(project_id)::text AND version = sqlc.arg(version)::text;

-- name: WeaveSnapshotOwnedProject :execrows
INSERT INTO weave_projects_archive (
    id, created_at, updated_at, system_name, ui_name, description, status,
    namespace, parent_project_id, staging_id, owner_id, visibility,
    deprecated, license, readme, topics, base_url, created_by_id, version_number
)
SELECT id, created_at, updated_at, system_name, ui_name, description,
    CASE WHEN status = 'deprecated' THEN status ELSE 'published' END,
    namespace, parent_project_id, staging_id, owner_id, visibility,
    deprecated, license, readme, topics, base_url, created_by_id, sqlc.arg(version)::text
FROM weave_projects p WHERE p.id = sqlc.arg(project_id)::text;

-- name: WeaveSnapshotOwnedInheritance :exec
INSERT INTO weave_project_inheritance_archive (
    project_id, parent_project_id, is_primary, canonical_order, adopted_at,
    source_mode, source_version, version_number
)
SELECT project_id, parent_project_id, is_primary, canonical_order, adopted_at,
    source_mode, source_version, sqlc.arg(version)::text
FROM weave_project_inheritance i WHERE i.project_id = sqlc.arg(project_id)::text;

-- name: WeaveSnapshotOwnedOntologyLinks :exec
INSERT INTO weave_project_ontology_versions_archive (
    project_id, ontology_version_id, added_at, added_by_id, is_primary, usage_notes, version_number
)
SELECT project_id, ontology_version_id, added_at, added_by_id, is_primary, usage_notes, sqlc.arg(version)::text
FROM weave_project_ontology_versions o WHERE o.project_id = sqlc.arg(project_id)::text;

-- name: WeaveSnapshotReleaseNamespaces :exec
INSERT INTO weave_release_namespace_bindings_archive (
    release_project_id, version_number, id, created_at, updated_at,
    airtable_raw, airtable_context, airtable_ref, semantic_id, long_semantic_id,
    system_name, ui_name, description, uri, project_id, prefix, namespace, weight, source, ontology_id
)
SELECT sqlc.arg(project_id)::text, sqlc.arg(version)::text, id, created_at, updated_at,
    airtable_raw, airtable_context, airtable_ref, semantic_id, long_semantic_id,
    system_name, ui_name, description, uri, project_id, prefix, namespace, weight, source, ontology_id
FROM weave_namespace_bindings
WHERE project_id = sqlc.arg(project_id)::text OR project_id IS NULL OR project_id = '';

-- name: WeaveListReleaseNamespaceBindings :many
SELECT * FROM weave_release_namespace_bindings_archive
WHERE release_project_id = sqlc.arg(project_id)::text AND version_number = sqlc.arg(version)::text
ORDER BY weight, prefix, namespace, id;

-- name: WeaveListReleaseParentTargets :many
SELECT * FROM weave_project_inheritance_archive
WHERE project_id = sqlc.arg(project_id)::text AND version_number = sqlc.arg(version)::text
ORDER BY is_primary DESC, canonical_order, parent_project_id;

-- name: WeaveUnverifiedReleaseParents :many
SELECT i.parent_project_id
FROM weave_project_inheritance i
LEFT JOIN weave_releases r ON r.project_id = i.parent_project_id AND r.version = i.source_version
WHERE i.project_id = sqlc.arg(project_id)::text
  AND (i.source_mode <> 'release' OR COALESCE(i.source_version, '') = '' OR COALESCE(r.archive_ownership_version, 0) <> 1)
UNION
SELECT p.parent_project_id FROM weave_projects p
WHERE p.id = sqlc.arg(project_id)::text AND p.parent_project_id IS NOT NULL AND p.parent_project_id <> ''
  AND NOT EXISTS (SELECT 1 FROM weave_project_inheritance i WHERE i.project_id = p.id)
ORDER BY parent_project_id;

-- name: WeaveAuditReleaseArchiveOwnership :many
SELECT 'unverified_release'::text AS issue, r.project_id, r.version, r.created_at,
    (SELECT count(*) FROM weave_releases other WHERE other.version = r.version AND other.project_id <> r.project_id) AS same_version_projects,
    EXISTS (
        SELECT 1 FROM weave_project_inheritance_archive i
        WHERE i.parent_project_id = r.project_id AND i.version_number = r.version AND i.project_id <> r.project_id
    ) AS possible_parent_key_collision
FROM weave_releases r
WHERE r.archive_ownership_version = 0
UNION ALL
SELECT 'orphan_project_archive'::text AS issue, p.id AS project_id, p.version_number AS version, p.created_at,
    (SELECT count(*) FROM weave_releases other WHERE other.version = p.version_number AND other.project_id <> p.id) AS same_version_projects,
    EXISTS (
        SELECT 1 FROM weave_project_inheritance_archive i
        WHERE i.parent_project_id = p.id AND i.version_number = p.version_number AND i.project_id <> p.id
    ) AS possible_parent_key_collision
FROM weave_projects_archive p
WHERE NOT EXISTS (SELECT 1 FROM weave_releases r WHERE r.project_id = p.id AND r.version = p.version_number)
ORDER BY created_at, project_id, version;
