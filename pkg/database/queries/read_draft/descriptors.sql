-- name: DraftActorDescriptors :many
WITH refs AS (
 SELECT owner_id AS actor_id FROM weave_projects WHERE id=sqlc.arg(project_id)::text
 UNION SELECT created_by_id FROM weave_projects WHERE id=sqlc.arg(project_id)::text
 UNION SELECT created_by_id FROM weave_adoptions WHERE project_id=sqlc.arg(project_id)::text
 UNION SELECT created_by_id FROM weave_entity_forks WHERE project_id=sqlc.arg(project_id)::text
 UNION SELECT actor_id FROM weave_project_attributions WHERE project_id=sqlc.arg(project_id)::text
 UNION SELECT actor_id FROM weave_project_actors WHERE project_id=sqlc.arg(project_id)::text
 UNION SELECT added_by_id FROM weave_project_ontology_versions WHERE project_id=sqlc.arg(project_id)::text
)
SELECT r.actor_id::text AS actor_id,(a.id IS NOT NULL)::boolean AS present,
 COALESCE(a.type,'')::text AS type,COALESCE(a.slug,'')::text AS slug,COALESCE(a.display_name,'')::text AS display_name
FROM refs r LEFT JOIN weave_actors a ON a.id=r.actor_id
WHERE r.actor_id IS NOT NULL AND btrim(r.actor_id)<>'' ORDER BY r.actor_id;
