-- name: ProjectOntologyInventory :one
SELECT ontology_dependencies_version=1 AS verified FROM weave_releases
WHERE project_id = @project_id::text AND version = @version::text;

-- name: ProjectOntologyArtifacts :many
SELECT r.artifact,r.digest,p.ontology_id,p.ontology_version_id,
       (l.ontology_version_id IS NOT NULL)::boolean AS linked,COALESCE(l.is_primary,false)::boolean AS is_primary
FROM weave_release_ontology_dependencies p
JOIN weave_ontology_releases r ON (r.ontology_id,r.version_id,r.digest)=(p.ontology_id,p.ontology_version_id,p.digest)
LEFT JOIN weave_project_ontology_versions_archive l ON l.project_id=p.project_id AND l.version_number=p.version_number AND l.ontology_version_id=p.ontology_version_id
WHERE p.project_id = @project_id::text AND p.version_number = @version::text
ORDER BY p.ontology_id;
