-- name: ProjectOntologyCandidates :many
SELECT o.id AS ontology_id,o.prefix,COALESCE(v.id,'')::text AS version_id,
       (a.version_id IS NOT NULL)::boolean AS active,
       (r.version_id IS NOT NULL)::boolean AS published
FROM weave_ontologies o
LEFT JOIN weave_ontology_versions v ON v.ontology_id=o.id
LEFT JOIN weave_ontology_active_versions a ON a.ontology_id=o.id AND a.version_id=v.id
LEFT JOIN weave_ontology_releases r ON r.version_id=v.id
ORDER BY o.id,v.id;

-- name: ProjectOntologyPublication :one
SELECT artifact,digest FROM weave_ontology_releases WHERE version_id=sqlc.arg(version_id)::text;

-- name: DraftOntologySelections :many
WITH bodies AS (
 SELECT to_jsonb(f) AS body FROM weave_fields f WHERE project_id=sqlc.arg(project_id)::text
 UNION ALL SELECT to_jsonb(m) FROM weave_models m WHERE project_id=sqlc.arg(project_id)::text
 UNION ALL SELECT to_jsonb(c) FROM weave_collections c WHERE project_id=sqlc.arg(project_id)::text
), selected AS (
 SELECT ontology_version_id AS id FROM weave_project_ontology_versions WHERE project_id=sqlc.arg(project_id)::text
 UNION SELECT value #>> '{}' FROM bodies, LATERAL jsonb_path_query(body,'$.**.ontology_version_id') AS value
)
SELECT DISTINCT id::text FROM selected WHERE id IS NOT NULL AND btrim(id)<>'' ORDER BY id;
