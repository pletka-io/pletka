-- name: BrowserInventoryIndex :one
WITH entities AS (
 SELECT e.id, e.system_name, e.ui_name, e.description, e.status, e.ontology_scope, e.model_type AS model_type, ''::text AS category_id FROM weave_models_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='model'
 UNION ALL
 SELECT e.id, e.system_name, e.ui_name, e.description, e.status, e.ontology_scope, ''::text AS model_type, COALESCE(e.default_category_id,'') AS category_id FROM weave_collections_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='collection'
 UNION ALL
 SELECT e.id, e.system_name, e.ui_name, e.description, e.status, e.ontology_scope, ''::text AS model_type, COALESCE(sqlc.arg(categories)::jsonb->>e.id,'') AS category_id FROM weave_fields_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='field'
), eligible AS (
 SELECT e.*, COALESCE(sqlc.arg(origins)::jsonb->>e.id,'own') AS origin_kind,
 (CASE WHEN COALESCE(e.ontology_scope->>'prefix','')='' THEN COALESCE(e.ontology_scope->>'local_name','') ELSE (e.ontology_scope->>'prefix')||':'||COALESCE(e.ontology_scope->>'local_name','') END)||COALESCE((SELECT string_agg('/'||CASE WHEN COALESCE(t.element->>'prefix','')='' THEN COALESCE(t.element->>'local_name','') ELSE (t.element->>'prefix')||':'||COALESCE(t.element->>'local_name','') END,'' ORDER BY t.n) FROM jsonb_array_elements(NULLIF(e.ontology_scope->'additional_types','null'::jsonb)) WITH ORDINALITY t(element,n)),'') AS scope_name
 FROM entities e WHERE sqlc.arg(owned)::boolean OR e.id=ANY(sqlc.arg(ids)::text[])
), filtered AS (
 SELECT e.*, (CASE sqlc.arg(sort_by)::text WHEN 'name' THEN COALESCE(system_name,'')
 WHEN 'scope' THEN scope_name ELSE COALESCE(ui_name->>'en',system_name,'') END) COLLATE "C" AS sort_key
 FROM eligible e WHERE
 (sqlc.arg(search)::text='' OR strpos(lower(COALESCE(system_name,'')||' '||id),lower(sqlc.arg(search)::text))>0
 OR EXISTS(SELECT 1 FROM jsonb_each_text(NULLIF(ui_name,'null'::jsonb)) t WHERE strpos(lower(t.value),lower(sqlc.arg(search)::text))>0)
 OR EXISTS(SELECT 1 FROM jsonb_each_text(NULLIF(description,'null'::jsonb)) t WHERE strpos(lower(t.value),lower(sqlc.arg(search)::text))>0))
 AND (sqlc.arg(origin_filter)::text='' OR origin_kind=ANY(string_to_array(sqlc.arg(origin_filter)::text,',')))
 AND (sqlc.arg(scope_filter)::text='' OR scope_name=ANY(string_to_array(sqlc.arg(scope_filter)::text,',')))
 AND (sqlc.arg(category_filter)::text='' OR category_id=ANY(string_to_array(sqlc.arg(category_filter)::text,',')))
 AND (sqlc.arg(model_filter)::text='' OR model_type=ANY(string_to_array(sqlc.arg(model_filter)::text,',')))
 AND (sqlc.arg(status_filter)::text='' OR status=ANY(string_to_array(sqlc.arg(status_filter)::text,',')))
), page AS (
 SELECT id,sort_key FROM filtered
 ORDER BY CASE WHEN NOT sqlc.arg(descending)::boolean THEN sort_key END ASC,
 CASE WHEN sqlc.arg(descending)::boolean THEN sort_key END DESC,id COLLATE "C"
 LIMIT sqlc.arg(row_limit)::integer
)
SELECT (SELECT count(*) FROM filtered)::bigint AS total,
 (SELECT count(*) FROM eligible)::bigint AS available,
 COALESCE((SELECT jsonb_agg(to_jsonb(p)) FROM page p),'[]'::jsonb)::jsonb AS items,
 COALESCE((SELECT jsonb_agg(x.scope_name) FROM (SELECT DISTINCT scope_name FROM eligible WHERE scope_name<>'' ORDER BY scope_name LIMIT 1001) x),'[]'::jsonb)::jsonb AS scopes,
 COALESCE((SELECT jsonb_agg(x.category_id) FROM (SELECT DISTINCT category_id FROM eligible WHERE category_id<>'' ORDER BY category_id LIMIT 1001) x),'[]'::jsonb)::jsonb AS categories;

-- name: BrowserInventoryRows :many
SELECT e.id,jsonb_build_object('id',e.id,'system_name',e.system_name,'ui_name',e.ui_name,'description',e.description,'status',e.status,'project_id',e.project_id,'deprecated',e.deprecated,'created_at',e.created_at,'updated_at',e.updated_at,'ontology_scope',COALESCE((e.ontology_scope->>'prefix')||':'||(e.ontology_scope->>'local_name'),''),'scope_definition',e.ontology_scope,'model_type',e.model_type,'semantic_id',e.id) AS body FROM weave_models_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='model' AND e.id=ANY(sqlc.arg(ids)::text[])
UNION ALL
SELECT e.id,jsonb_build_object('id',e.id,'system_name',e.system_name,'ui_name',e.ui_name,'description',e.description,'status',e.status,'project_id',e.project_id,'deprecated',e.deprecated,'created_at',e.created_at,'updated_at',e.updated_at,'ontology_scope',COALESCE((e.ontology_scope->>'prefix')||':'||(e.ontology_scope->>'local_name'),''),'scope_definition',e.ontology_scope,'collection_number',e.collection_number,'canonical_collection_order',e.canonical_collection_order,'default_category_id',e.default_category_id,'semantic_id',e.id) AS body FROM weave_collections_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='collection' AND e.id=ANY(sqlc.arg(ids)::text[])
UNION ALL
SELECT e.id,jsonb_build_object('id',e.id,'system_name',e.system_name,'ui_name',e.ui_name,'description',e.description,'status',e.status,'project_id',e.project_id,'deprecated',e.deprecated,'created_at',e.created_at,'updated_at',e.updated_at,'ontology_scope',COALESCE((e.ontology_scope->>'prefix')||':'||(e.ontology_scope->>'local_name'),''),'scope_definition',e.ontology_scope,'expected_value_type',e.expected_value_type,'path_elements',e.path_elements,'semantic_id',e.semantic_id) AS body FROM weave_fields_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='field' AND e.id=ANY(sqlc.arg(ids)::text[]);

-- name: BrowserInventoryRelations :many
SELECT CASE WHEN COALESCE(o.entity_type,'')='' THEN 'field' ELSE o.entity_type END::text AS entity_kind,CASE WHEN COALESCE(o.entity_type,'')='' THEN o.field_id ELSE o.entity_id END::text AS entity_id,
 o.field_id,COALESCE(o.category_id,'')::text AS category_id,COALESCE(o.part_of_collection_id,'')::text AS collection_id,
 COALESCE(r.ref_type,'')::text AS ref_kind,COALESCE(NULLIF(r.target_id,''),r.semantic_id,'')::text AS ref_id
FROM weave_field_overrides_archive o LEFT JOIN weave_override_refs_archive r ON r.override_id=o.id AND r.version_number=o.version_number AND r.project_id=o.project_id
WHERE o.project_id=sqlc.arg(project_id)::text AND o.version_number=sqlc.arg(version)::text AND (sqlc.arg(all_entities)::boolean OR ((o.entity_type=sqlc.arg(kind)::text AND o.entity_id=ANY(sqlc.arg(ids)::text[])) OR (sqlc.arg(kind)::text='field' AND COALESCE(o.entity_type,'')='' AND o.field_id=ANY(sqlc.arg(ids)::text[]))))
UNION ALL
SELECT 'model',p.model_id,'',COALESCE(p.category_id,''),p.collection_id,'',''
FROM weave_collection_placements_archive p WHERE p.project_id=sqlc.arg(project_id)::text AND p.version_number=sqlc.arg(version)::text AND (sqlc.arg(all_entities)::boolean OR (sqlc.arg(kind)::text='model' AND p.model_id=ANY(sqlc.arg(ids)::text[])))
LIMIT 100001;

-- name: BrowserInventoryCategories :many
SELECT id,ui_name FROM weave_categories_archive c WHERE c.project_id=sqlc.arg(project_id)::text AND c.version_number=sqlc.arg(version)::text AND c.id=ANY(sqlc.arg(ids)::text[]);
