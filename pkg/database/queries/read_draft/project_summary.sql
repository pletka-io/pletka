-- name: BrowserProjectCounts :one
SELECT (SELECT count(*)::integer FROM weave_models WHERE project_id=sqlc.arg(project_id)::text) AS models,
       (SELECT count(*)::integer FROM weave_collections WHERE project_id=sqlc.arg(project_id)::text) AS collections,
       (SELECT count(*)::integer FROM weave_fields WHERE project_id=sqlc.arg(project_id)::text) AS fields,
       (SELECT count(*)::integer FROM weave_categories WHERE project_id=sqlc.arg(project_id)::text) AS categories,
       (SELECT count(*)::integer FROM weave_examples WHERE project_id=sqlc.arg(project_id)::text) AS examples,
       (SELECT count(*)::integer FROM weave_concept_lists WHERE project_id=sqlc.arg(project_id)::text) AS concept_lists,
       (SELECT count(*)::integer FROM weave_adoptions WHERE project_id=sqlc.arg(project_id)::text AND context_entity_type='project' AND context_entity_id=sqlc.arg(project_id)::text AND entity_type IN ('model','collection','field')) AS adoptions;

-- name: BrowserProjectCredits :many
SELECT c.actor_id,c.kind,c.position,c.note,
       COALESCE(a.display_name,'')::text AS actor_name,COALESCE(a.type,'')::text AS actor_type,
       (a.id IS NOT NULL)::boolean AS present
FROM weave_project_attributions c LEFT JOIN weave_actors a ON a.id=c.actor_id
WHERE c.project_id=sqlc.arg(project_id)::text
ORDER BY c.kind,c.position,c.actor_id LIMIT sqlc.arg(row_limit)::integer;

-- name: BrowserOwnOntologySummaries :many
WITH bodies AS (
 SELECT l.ontology_version_id, r.body AS body,r.digest AS digest
 FROM weave_project_ontology_versions l LEFT JOIN weave_ontology_body_seals r ON r.version_id=l.ontology_version_id
 WHERE l.project_id=sqlc.arg(project_id)::text
)
SELECT ontology_version_id,
 jsonb_build_object('digest',digest,'name',body->'ontology'->>'name','label',body->'version'->'ontology_label',
 'version',body->'version'->>'version_string','uri',body->'version'->>'ontology_uri',
 'classes',jsonb_array_length(body->'classes'),'properties',jsonb_array_length(body->'properties')) AS metadata,
 COALESCE(body->'version'->>'id'=ontology_version_id AND body->>'format_version'='1',false)::boolean AS present
FROM bodies ORDER BY ontology_version_id LIMIT sqlc.arg(row_limit)::integer;

-- name: BrowserOntologyReferences :many
WITH elements AS (
 SELECT ontology_scope AS element,'class'::text AS default_kind FROM weave_fields WHERE project_id=sqlc.arg(project_id)::text
 UNION ALL SELECT ontology_scope,'class' FROM weave_models WHERE project_id=sqlc.arg(project_id)::text
 UNION ALL SELECT ontology_scope,'class' FROM weave_collections WHERE project_id=sqlc.arg(project_id)::text
 UNION ALL SELECT p.element,'' FROM weave_fields f CROSS JOIN LATERAL jsonb_array_elements(NULLIF(f.path_elements,'null'::jsonb)) p(element) WHERE f.project_id=sqlc.arg(project_id)::text
 UNION ALL SELECT p.element,'' FROM weave_fields f CROSS JOIN LATERAL jsonb_array_elements(NULLIF(f.subfield_paths,'null'::jsonb)) sub(element)
 CROSS JOIN LATERAL jsonb_array_elements(NULLIF(sub.element->'path_elements','null'::jsonb)) p(element) WHERE f.project_id=sqlc.arg(project_id)::text
), refs AS (
 SELECT DISTINCT r.reference::jsonb AS reference FROM elements e CROSS JOIN LATERAL weave_semantic_element_references(e.element,e.default_kind,'') AS r(location,reference)
)
SELECT reference::jsonb AS reference FROM refs ORDER BY reference LIMIT 10001;

-- name: BrowserMatchOntologyReferences :many
WITH body AS (
 SELECT version_id,body AS b FROM weave_ontology_body_seals WHERE version_id=ANY(sqlc.arg(ontology_versions)::text[])
), terms AS (
 SELECT version_id,'class'::text AS kind,t FROM body,LATERAL jsonb_array_elements(b->'classes') t
 UNION ALL SELECT version_id,'property',t FROM body,LATERAL jsonb_array_elements(b->'properties') t
), matches AS (
 SELECT DISTINCT version_id,kind,t->>'uri' AS uri FROM terms,LATERAL jsonb_array_elements(sqlc.arg(references_json)::jsonb) AS refs(reference)
 WHERE kind=reference->>'kind' AND (COALESCE(reference->>'version_id','')='' OR reference->>'version_id'=terms.version_id)
 AND (CASE WHEN COALESCE(reference->>'uri','')<>'' THEN t->>'uri'=reference->>'uri'
 ELSE t->>'prefix'=reference->>'prefix' AND t->>'local_name'=reference->>'local_name' END)
)
SELECT selected.version_id::text AS version_id,count(DISTINCT uri) FILTER(WHERE kind='class')::integer AS classes,
       count(DISTINCT uri) FILTER(WHERE kind='property')::integer AS properties
FROM unnest(sqlc.arg(ontology_versions)::text[]) selected(version_id) LEFT JOIN matches ON matches.version_id=selected.version_id
GROUP BY selected.version_id ORDER BY selected.version_id;
