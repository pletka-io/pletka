-- name: BrowserProjectCounts :one
SELECT (SELECT count(*)::integer FROM weave_models_archive WHERE project_id=sqlc.arg(project_id)::text AND version_number=sqlc.arg(version)::text) AS models,
       (SELECT count(*)::integer FROM weave_collections_archive WHERE project_id=sqlc.arg(project_id)::text AND version_number=sqlc.arg(version)::text) AS collections,
       (SELECT count(*)::integer FROM weave_fields_archive WHERE project_id=sqlc.arg(project_id)::text AND version_number=sqlc.arg(version)::text) AS fields,
       (SELECT count(*)::integer FROM weave_categories_archive WHERE project_id=sqlc.arg(project_id)::text AND version_number=sqlc.arg(version)::text) AS categories,
       (SELECT count(*)::integer FROM weave_examples_archive WHERE project_id=sqlc.arg(project_id)::text AND version_number=sqlc.arg(version)::text) AS examples,
       (SELECT count(*)::integer FROM weave_concept_lists_archive WHERE project_id=sqlc.arg(project_id)::text AND version_number=sqlc.arg(version)::text) AS concept_lists,
       (SELECT count(*)::integer FROM weave_adoptions_archive WHERE project_id=sqlc.arg(project_id)::text AND version_number=sqlc.arg(version)::text AND context_entity_type='project' AND context_entity_id=sqlc.arg(project_id)::text AND entity_type IN ('model','collection','field')) AS adoptions;

-- name: BrowserProjectCredits :many
SELECT c.actor_id,c.kind,c.position,c.note,
       COALESCE(r.document->'actors'->c.actor_id->>'display_name','')::text AS actor_name,
       COALESCE(r.document->'actors'->c.actor_id->>'type','')::text AS actor_type,
       COALESCE(r.document->'actors'->c.actor_id->>'actor_id'=c.actor_id,false)::boolean AS present
FROM weave_project_attributions_archive c
LEFT JOIN weave_release_descriptors r ON r.project_id=c.project_id AND r.version_number=c.version_number
WHERE c.project_id=sqlc.arg(project_id)::text AND c.version_number=sqlc.arg(version)::text
ORDER BY c.kind,c.position,c.actor_id LIMIT sqlc.arg(row_limit)::integer;
