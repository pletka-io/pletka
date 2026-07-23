-- name: WeaveCreateOntologyVersion :one
INSERT INTO weave_ontology_versions (
    id, ontology_id, version_string, is_active, compatible_base_versions,
    rdf_content, parsed_at, original_filename, file_size, file_md5,
    ontology_uri, version_iri, version_info, imported_ontologies,
    ontology_label, ontology_comment, ontology_metadata,
    class_count, property_count
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18, $19
)
RETURNING *;

-- name: WeaveGetOntologyVersionByID :one
SELECT * FROM weave_ontology_versions WHERE id = $1;

-- name: WeaveGetOntologyVersionByOntologyAndString :one
SELECT * FROM weave_ontology_versions
WHERE ontology_id = $1 AND version_string = $2;

-- name: WeaveGetActiveOntologyVersion :one
SELECT * FROM weave_ontology_versions
WHERE ontology_id = $1 AND is_active = true;

-- name: WeaveListOntologyVersionsByOntology :many
SELECT * FROM weave_ontology_versions
WHERE ontology_id = $1
ORDER BY parsed_at DESC NULLS LAST, version_string DESC;

-- name: WeaveUpdateOntologyVersionMetadata :one
UPDATE weave_ontology_versions
SET version_string = $2,
    compatible_base_versions = $3,
    ontology_uri = $4,
    version_iri = $5,
    version_info = $6,
    imported_ontologies = $7,
    ontology_label = $8,
    ontology_comment = $9,
    ontology_metadata = $10,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: WeaveUpdateOntologyVersionCounts :exec
UPDATE weave_ontology_versions
SET class_count = $2,
    property_count = $3,
    updated_at = now()
WHERE id = $1;

-- name: WeaveSetActiveOntologyVersion :exec
-- Atomically clear any existing active version for this ontology then
-- set the new one. The partial unique index
-- idx_weave_ontology_versions_active_per_ontology enforces at most one
-- active version per ontology.
WITH cleared AS (
    UPDATE weave_ontology_versions
    SET is_active = false,
        updated_at = now()
    WHERE weave_ontology_versions.ontology_id = sqlc.arg(ontology_id)
      AND is_active = true
)
UPDATE weave_ontology_versions
SET is_active = true,
    updated_at = now()
WHERE weave_ontology_versions.ontology_id = sqlc.arg(ontology_id)
  AND weave_ontology_versions.id = sqlc.arg(target_id);

-- name: WeaveDeleteOntologyVersion :exec
DELETE FROM weave_ontology_versions WHERE id = $1;

-- name: WeaveOntologyVersionIsUsedByProjects :one
SELECT EXISTS (
    SELECT 1 FROM weave_project_ontology_versions
    WHERE ontology_version_id = $1
)::bool;

-- name: WeaveCountProjectsUsingOntologyVersion :one
SELECT count(*)::bigint
FROM weave_project_ontology_versions
WHERE ontology_version_id = $1;

-- name: WeaveListProjectsUsingOntologyVersion :many
SELECT pov.project_id,
       p.ui_name,
       p.system_name,
       pov.added_at,
       pov.is_primary
FROM weave_project_ontology_versions pov
LEFT JOIN weave_projects p ON p.id = pov.project_id
WHERE pov.ontology_version_id = $1
ORDER BY pov.added_at DESC
LIMIT $2;

-- name: WeaveOntologyVersionUsageCounts :many
-- Per-version usage count for one ontology — used by the version list
-- view to render "linked to N projects" badges in one query.
SELECT v.id AS version_id,
       count(pov.project_id)::bigint AS project_count
FROM weave_ontology_versions v
LEFT JOIN weave_project_ontology_versions pov
       ON pov.ontology_version_id = v.id
WHERE v.ontology_id = $1
GROUP BY v.id;

-- name: WeaveUpsertOntologyVersionCompanion :exec
INSERT INTO weave_ontology_version_companions (ontology_version_id, filename, description, content, position)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (ontology_version_id, filename) DO UPDATE SET
    description = EXCLUDED.description,
    content = EXCLUDED.content,
    position = EXCLUDED.position;

-- name: WeaveListOntologyVersionCompanions :many
SELECT ontology_version_id, filename, description, content, position
FROM weave_ontology_version_companions
WHERE ontology_version_id = $1
ORDER BY position, filename;
