-- name: WeaveListProjectOntologyVersions :many
SELECT * FROM weave_project_ontology_versions
WHERE project_id = $1
ORDER BY is_primary DESC, added_at ASC;

-- name: WeaveGetProjectOntologyVersion :one
SELECT * FROM weave_project_ontology_versions
WHERE project_id = $1 AND ontology_version_id = $2;

-- name: WeaveCreateProjectOntologyVersion :one
INSERT INTO weave_project_ontology_versions (
    project_id, ontology_version_id, added_by_id,
    is_primary, usage_notes, added_at
) VALUES ($1, $2, $3, $4, $5, now())
RETURNING *;

-- name: WeaveUpdateProjectOntologyVersion :one
UPDATE weave_project_ontology_versions
SET is_primary = $3,
    usage_notes = $4
WHERE project_id = $1 AND ontology_version_id = $2
RETURNING *;

-- name: WeaveDeleteProjectOntologyVersion :exec
DELETE FROM weave_project_ontology_versions
WHERE project_id = $1 AND ontology_version_id = $2;

-- name: WeaveClearProjectPrimaryOntologies :exec
UPDATE weave_project_ontology_versions
SET is_primary = false
WHERE project_id = $1 AND is_primary = true;

-- name: WeaveSetProjectPrimaryOntology :exec
UPDATE weave_project_ontology_versions
SET is_primary = true
WHERE project_id = $1 AND ontology_version_id = $2;

-- name: WeaveCountPathElementsForVersionInProject :one
-- Used as the delete-usage guard in Task 19.
SELECT COUNT(*) FROM weave_path_elements wpe
JOIN weave_fields wf ON wpe.field_id = wf.id
WHERE wpe.ontology_version_id = $1 AND wf.project_id = $2;

-- name: WeaveSamplePathElementFieldsForVersion :many
-- Returns up to 10 field samples referencing the version; surfaced in 409 payloads.
SELECT DISTINCT wf.id, wf.semantic_id, wf.system_name, wf.ui_name
FROM weave_path_elements wpe
JOIN weave_fields wf ON wpe.field_id = wf.id
WHERE wpe.ontology_version_id = $1 AND wf.project_id = $2
LIMIT 10;

-- name: WeaveListProjectOntologyBundle :many
-- Project's linked ontologies with the raw schema content, used to build
-- the X3ML <target> blocks and the downloadable ZIP bundle.
SELECT
    o.prefix,
    o.namespace,
    o.name AS ontology_name,
    ov.version_string,
    ov.original_filename,
    ov.rdf_content
FROM weave_project_ontology_versions pov
JOIN weave_ontology_versions ov ON ov.id = pov.ontology_version_id
JOIN weave_ontologies o ON o.id = ov.ontology_id
WHERE pov.project_id = $1
ORDER BY pov.is_primary DESC, o.prefix ASC;

-- name: WeaveListOntologyBundleByVersionIDs :many
-- Bundle fields for an explicit set of ontology-version IDs (the resolved
-- own+inherited set), used to build inheritance-aware X3ML zip + targets.
SELECT
    o.prefix,
    o.namespace,
    o.name AS ontology_name,
    ov.version_string,
    ov.original_filename,
    ov.rdf_content
FROM weave_ontology_versions ov
JOIN weave_ontologies o ON o.id = ov.ontology_id
WHERE ov.id = ANY($1::text[])
ORDER BY o.prefix ASC;

