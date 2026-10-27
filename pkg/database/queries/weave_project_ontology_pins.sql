-- name: WeaveProjectOntologySelections :many
WITH bodies AS (
    SELECT to_jsonb(f) AS body FROM weave_fields_archive f WHERE project_id= @project_id::text AND version_number= @version::text
    UNION ALL SELECT to_jsonb(m) FROM weave_models_archive m WHERE project_id= @project_id::text AND version_number= @version::text
    UNION ALL SELECT to_jsonb(c) FROM weave_collections_archive c WHERE project_id= @project_id::text AND version_number= @version::text
), selected AS (
    SELECT ontology_version_id AS id FROM weave_project_ontology_versions_archive WHERE project_id= @project_id::text AND version_number= @version::text
    UNION SELECT value #>> '{}' FROM bodies, LATERAL jsonb_path_query(body,'$.**.ontology_version_id') AS value
)
SELECT DISTINCT id::text FROM selected WHERE id IS NOT NULL AND btrim(id)<>'' ORDER BY id;

-- name: WeavePublishedOntologyIdentity :one
SELECT ontology_id,version_id,digest FROM weave_ontology_releases WHERE version_id=$1;

-- name: WeaveProjectOntologySourceInventories :many
SELECT d.source_project_id,d.source_version,r.ontology_dependencies_version,
    EXISTS(SELECT 1 FROM weave_release_ontology_references s WHERE s.project_id=d.source_project_id AND s.version_number=d.source_version)::boolean AS semantic_verified
FROM weave_release_project_dependencies d JOIN weave_releases r ON r.project_id=d.source_project_id AND r.version=d.source_version
WHERE d.project_id= @project_id::text AND d.version_number= @version::text;

-- name: WeaveProjectOntologySourcePins :many
SELECT p.ontology_id,p.ontology_version_id,p.digest FROM weave_release_project_dependencies d
JOIN weave_release_ontology_dependencies p ON p.project_id=d.source_project_id AND p.version_number=d.source_version
WHERE d.project_id= @project_id::text AND d.version_number= @version::text;

-- name: WeaveInsertProjectOntologyPin :exec
INSERT INTO weave_release_ontology_dependencies(project_id,version_number,ontology_id,ontology_version_id,digest)
VALUES (@project_id,@version,@ontology_id,@ontology_version_id,@digest);

-- name: WeaveMarkProjectOntologyInventory :exec
UPDATE weave_releases SET ontology_dependencies_version=1 WHERE project_id= @project_id::text AND version= @version::text;

-- name: WeaveProjectSemanticReferences :many
SELECT entity_kind::text,entity_id::text,location::text,reference::jsonb
FROM weave_project_semantic_references
WHERE project_id = sqlc.arg(project_id)::text AND version_number = sqlc.arg(version)::text
ORDER BY entity_kind,entity_id,location;

-- name: WeaveCaptureProjectSemanticReferences :exec
INSERT INTO weave_release_ontology_references(project_id,version_number,resolutions)
VALUES(sqlc.arg(project_id),sqlc.arg(version),sqlc.arg(resolutions));
