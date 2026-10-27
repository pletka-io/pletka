-- name: WeaveReleaseDescriptorActors :many
SELECT r.actor_id::text AS actor_id, (a.id IS NOT NULL)::boolean AS present,
    COALESCE(a.type,'')::text AS type, COALESCE(a.slug,'')::text AS slug,
    COALESCE(a.display_name,'')::text AS display_name
FROM weave_release_actor_references r LEFT JOIN weave_actors a ON a.id=r.actor_id
WHERE r.project_id=sqlc.arg(project_id)::text AND r.version_number=sqlc.arg(version)::text
  AND r.actor_id IS NOT NULL AND btrim(r.actor_id)<>'' ORDER BY r.actor_id;

-- name: WeaveReleaseDescriptorOwner :one
SELECT owner_id FROM weave_projects_archive WHERE id=sqlc.arg(project_id)::text AND version_number=sqlc.arg(version)::text;

-- name: WeaveReleaseDescriptorSources :many
SELECT p.source_project_id,p.source_version,d.document
FROM weave_release_project_dependencies p
LEFT JOIN weave_release_descriptors d ON d.project_id=p.source_project_id AND d.version_number=p.source_version
WHERE p.project_id=sqlc.arg(project_id)::text AND p.version_number=sqlc.arg(version)::text ORDER BY p.source_project_id;

-- name: WeaveReleaseDescriptorOntologies :many
SELECT r.artifact,r.digest,p.ontology_id,p.ontology_version_id
FROM weave_release_ontology_dependencies p
JOIN weave_ontology_releases r ON (r.ontology_id,r.version_id,r.digest)=(p.ontology_id,p.ontology_version_id,p.digest)
WHERE p.project_id=sqlc.arg(project_id)::text AND p.version_number=sqlc.arg(version)::text ORDER BY p.ontology_id;

-- name: WeaveInsertReleaseDescriptors :exec
INSERT INTO weave_release_descriptors(project_id,version_number,document) VALUES (sqlc.arg(project_id),sqlc.arg(version),sqlc.arg(document));
