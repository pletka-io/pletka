-- Compact reference identities only; no entry bodies or example values.
-- name: BrowserCatalogReferences :many
SELECT DISTINCT kind,id FROM (
 SELECT 'vocabulary'::text AS kind,e.vocabulary_id AS id FROM weave_concept_lists_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='concept_list'
 UNION ALL SELECT 'vocabulary_entry',e.list_type FROM weave_concept_lists_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='concept_list'
 UNION ALL SELECT e.entity_type,e.entity_id FROM weave_examples_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='example'
) refs WHERE id IS NOT NULL AND id<>'' ORDER BY kind,id LIMIT 10001;

-- name: BrowserCatalogLabels :many
SELECT e.id,COALESCE(e.ui_name,'{}'::jsonb)::jsonb AS label,COALESCE(e.system_name,e.id)::text AS fallback,''::text AS uri FROM weave_vocabularies_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='vocabulary' AND e.id=ANY(sqlc.arg(ids)::text[])
UNION ALL SELECT e.id,COALESCE(e.label,'{}'::jsonb)::jsonb,e.uri,e.uri FROM weave_vocabulary_entries_archive e JOIN weave_vocabularies_archive p ON p.id=e.vocabulary_id AND p.version_number=e.version_number WHERE p.project_id=sqlc.arg(project_id)::text AND p.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='vocabulary_entry' AND e.id=ANY(sqlc.arg(ids)::text[])
UNION ALL SELECT e.id,COALESCE(e.ui_name,'{}'::jsonb)::jsonb,COALESCE(e.system_name,e.id)::text,''::text FROM weave_models_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='model' AND e.id=ANY(sqlc.arg(ids)::text[])
UNION ALL SELECT e.id,COALESCE(e.ui_name,'{}'::jsonb)::jsonb,COALESCE(e.system_name,e.id)::text,''::text FROM weave_collections_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='collection' AND e.id=ANY(sqlc.arg(ids)::text[]);

-- Labels were resolved through this session's selected dependency graph.
-- name: BrowserCatalogPage :one
WITH entities AS (
 SELECT e.id,e.project_id,COALESCE(e.semantic_id,'') AS semantic_id,COALESCE(e.system_name,'') AS system_name,e.ui_name,e.description,e.status,e.created_at,e.updated_at,
 COALESCE(e.vocabulary_id,'') AS vocabulary_id,COALESCE(e.list_type,'') AS list_type,e.is_closed,
 (SELECT count(*) FROM weave_concept_list_entries_archive ce WHERE ce.concept_list_id=e.id AND ce.version_number=e.version_number)::bigint AS entry_count,
 (SELECT count(DISTINCT o.field_id) FROM weave_field_overrides_archive o JOIN weave_override_refs_archive r ON r.override_id=o.id AND r.version_number=o.version_number WHERE o.project_id=e.project_id AND o.version_number=e.version_number AND r.ref_type='concept_list' AND (r.target_id=e.id OR (COALESCE(r.target_id,'')='' AND NULLIF(r.semantic_id,'')=NULLIF(e.semantic_id,''))))::bigint AS bound_field_count,
 ''::text AS entity_type,''::text AS entity_id
 FROM weave_concept_lists_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='concept_list'
 UNION ALL
 SELECT e.id,e.project_id,''::text,''::text,e.title,e.description,e.status,e.created_at,e.updated_at,''::text,''::text,false,0::bigint,0::bigint,e.entity_type,e.entity_id
 FROM weave_examples_archive e WHERE e.project_id=sqlc.arg(project_id)::text AND e.version_number=sqlc.arg(version)::text AND sqlc.arg(kind)::text='example'
), decorated AS (
 SELECT e.*,
 COALESCE(sqlc.arg(labels)::jsonb->('vocabulary:'||vocabulary_id)->>'name','') AS vocabulary_label,
 COALESCE(sqlc.arg(labels)::jsonb->('vocabulary_entry:'||list_type)->>'name','') AS list_type_label,
 COALESCE(sqlc.arg(labels)::jsonb->('vocabulary_entry:'||list_type)->>'uri','') AS list_type_uri,
 COALESCE(sqlc.arg(labels)::jsonb->(entity_type||':'||entity_id)->>'name','') AS entity_name,
 COALESCE(sqlc.arg(labels)::jsonb->(entity_type||':'||entity_id)->'labels','{}'::jsonb)::jsonb AS entity_names
 FROM entities e
), filtered AS (
 SELECT e.* FROM decorated e WHERE
 (sqlc.arg(search)::text='' OR strpos(lower(id||' '||semantic_id||' '||system_name||' '||status||' '||vocabulary_label||' '||list_type_label||' '||list_type_uri),lower(sqlc.arg(search)::text))>0
 OR EXISTS(SELECT 1 FROM jsonb_each_text(NULLIF(ui_name,'null'::jsonb)) t WHERE strpos(lower(t.value),lower(sqlc.arg(search)::text))>0)
 OR EXISTS(SELECT 1 FROM jsonb_each_text(NULLIF(description,'null'::jsonb)) t WHERE strpos(lower(t.value),lower(sqlc.arg(search)::text))>0))
 AND (sqlc.arg(status_filter)::text='' OR status=ANY(string_to_array(sqlc.arg(status_filter)::text,',')))
 AND (sqlc.arg(vocabulary_filter)::text='' OR vocabulary_id=ANY(string_to_array(sqlc.arg(vocabulary_filter)::text,',')))
 AND (sqlc.arg(entity_type)::text='' OR entity_type=sqlc.arg(entity_type)::text)
 AND (sqlc.arg(entity_id)::text='' OR entity_id=sqlc.arg(entity_id)::text)
), keyed AS (
 SELECT e.*,CASE sqlc.arg(sort_by)::text WHEN 'semantic_id' THEN semantic_id WHEN 'system_name' THEN system_name WHEN 'status' THEN status WHEN 'vocabulary' THEN vocabulary_label WHEN 'entries' THEN '' WHEN 'bound_fields' THEN '' WHEN 'created_at' THEN '' WHEN 'updated_at' THEN '' ELSE COALESCE((SELECT string_agg(t.value,' ' ORDER BY t.value COLLATE "C") FROM jsonb_each_text(NULLIF(ui_name,'null'::jsonb)) t),'') END COLLATE "C" AS text_key,
 CASE sqlc.arg(sort_by)::text WHEN 'entries' THEN entry_count WHEN 'bound_fields' THEN bound_field_count ELSE 0 END AS number_key,
 CASE sqlc.arg(sort_by)::text WHEN 'created_at' THEN created_at WHEN 'updated_at' THEN updated_at ELSE NULL END AS time_key
 FROM filtered e
), page AS (
 SELECT e.* FROM keyed e ORDER BY
 CASE WHEN NOT sqlc.arg(descending)::boolean THEN time_key END ASC,CASE WHEN sqlc.arg(descending)::boolean THEN time_key END DESC,
 CASE WHEN NOT sqlc.arg(descending)::boolean THEN number_key END ASC,CASE WHEN sqlc.arg(descending)::boolean THEN number_key END DESC,
 CASE WHEN NOT sqlc.arg(descending)::boolean THEN text_key END ASC,CASE WHEN sqlc.arg(descending)::boolean THEN text_key END DESC,
 CASE WHEN sqlc.arg(kind)::text='example' AND sqlc.arg(sort_by)::text='status' THEN updated_at END DESC,
 CASE WHEN NOT sqlc.arg(descending)::boolean THEN semantic_id END COLLATE "C" ASC,
 CASE WHEN sqlc.arg(descending)::boolean THEN semantic_id END COLLATE "C" DESC,id COLLATE "C"
 LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::bigint
)
SELECT (SELECT count(*) FROM filtered)::bigint AS total,
 COALESCE((SELECT jsonb_agg(to_jsonb(p)) FROM page p),'[]'::jsonb)::jsonb AS items,
 COALESCE((SELECT jsonb_agg(to_jsonb(v)) FROM (SELECT DISTINCT vocabulary_id,vocabulary_label FROM decorated WHERE vocabulary_id<>'' ORDER BY vocabulary_id LIMIT 1001) v),'[]'::jsonb)::jsonb AS vocabularies;
