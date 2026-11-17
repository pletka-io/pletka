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


-- name: WeaveCountBlockingOntologyUsage :one
-- The unlink guard: fields, models and collections of the project that name a
-- term of the version which no linked version outside the removal provides.
-- A project can link two versions of one ontology, and an import can carry
-- terms of another ontology; a term that stays available does not block.
WITH kept_qnames AS (
    SELECT oc.qname
    FROM weave_ontology_classes oc
    JOIN weave_project_ontology_versions wpov ON wpov.ontology_version_id = oc.ontology_version_id
    WHERE wpov.project_id = sqlc.arg(project_id) AND wpov.version_number = ''
      AND wpov.ontology_version_id <> ALL(sqlc.arg(removing)::text[])
    UNION
    SELECT op.qname
    FROM weave_ontology_properties op
    JOIN weave_project_ontology_versions wpov ON wpov.ontology_version_id = op.ontology_version_id
    WHERE wpov.project_id = sqlc.arg(project_id) AND wpov.version_number = ''
      AND wpov.ontology_version_id <> ALL(sqlc.arg(removing)::text[])
), version_qnames AS (
    SELECT oc.qname FROM weave_ontology_classes oc
    LEFT JOIN kept_qnames k ON k.qname = oc.qname
    WHERE oc.ontology_version_id = sqlc.arg(version_id) AND k.qname IS NULL
    UNION
    SELECT op.qname FROM weave_ontology_properties op
    LEFT JOIN kept_qnames k ON k.qname = op.qname
    WHERE op.ontology_version_id = sqlc.arg(version_id) AND k.qname IS NULL
), entities_using AS (
    SELECT 'field:' || f.id AS entity_key
    FROM weave_fields f,
         LATERAL jsonb_array_elements(COALESCE(f.path_elements, '[]'::jsonb)) AS elem
    WHERE f.project_id = sqlc.arg(project_id)
      AND ((elem->>'prefix') || ':' || (elem->>'local_name')) IN (SELECT vq.qname FROM version_qnames vq)
    UNION
    SELECT 'field:' || f.id
    FROM weave_fields f
    WHERE f.project_id = sqlc.arg(project_id)
      AND ((f.ontology_scope->>'prefix') || ':' || (f.ontology_scope->>'local_name')) IN (SELECT vq.qname FROM version_qnames vq)
    UNION
    SELECT 'model:' || m.id
    FROM weave_models m
    WHERE m.project_id = sqlc.arg(project_id)
      AND ((m.ontology_scope->>'prefix') || ':' || (m.ontology_scope->>'local_name')) IN (SELECT vq.qname FROM version_qnames vq)
    UNION
    SELECT 'collection:' || c.id
    FROM weave_collections c
    WHERE c.project_id = sqlc.arg(project_id)
      AND ((c.ontology_scope->>'prefix') || ':' || (c.ontology_scope->>'local_name')) IN (SELECT vq.qname FROM version_qnames vq)
)
SELECT COUNT(*)::bigint FROM entities_using;
