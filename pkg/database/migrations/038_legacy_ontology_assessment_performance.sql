-- +goose Up
-- Preserve legacy evidence bytes while avoiding repeated parsing of full RDF
-- bodies and repeated assessment of identical normalized references.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_legacy_ontology_assessment(ref jsonb, inventory jsonb, aliases jsonb) RETURNS jsonb
LANGUAGE sql IMMUTABLE AS $$
    WITH bodies AS MATERIALIZED (
        -- Parsing a sealed body also parses its RDF and companions. Do it once
        -- per body, never once per candidate term or matching predicate.
        SELECT b->>'ontology_id' ontology_id,b->>'version_id' version_id,b->>'digest' digest,
            (b->>'body_json')::jsonb AS body
        FROM jsonb_array_elements(inventory) b
    ), candidates AS (
        SELECT DISTINCT b.ontology_id,b.version_id,b.digest,t->>'uri' uri
        FROM bodies b
        CROSS JOIN LATERAL jsonb_array_elements(b.body ->
            CASE ref->>'kind' WHEN 'class' THEN 'classes' WHEN 'property' THEN 'properties' ELSE '' END) t
        WHERE (ref->>'version_id'='' OR ref->>'version_id'=b.version_id)
          AND ((ref->>'prefix'='')=(ref->>'local_name'=''))
          AND (ref->>'uri'<>'' OR ref->>'prefix'<>'')
          AND (ref->>'uri'='' OR weave_semantic_term_matches(ref->>'uri',t,aliases || COALESCE(b.body->'namespace_bindings','[]')))
          AND (ref->>'prefix'='' OR weave_semantic_term_matches((ref->>'prefix') || ':' || (ref->>'local_name'),t,aliases || COALESCE(b.body->'namespace_bindings','[]')))
    ), matches AS (
        SELECT COALESCE(jsonb_agg(to_jsonb(c) ORDER BY version_id COLLATE "C",uri COLLATE "C"),'[]') items FROM candidates c
    ), validity AS (
        SELECT ref->>'kind' NOT IN ('class','property')
            OR ((ref->>'prefix'='')<>(ref->>'local_name'=''))
            OR (ref->>'uri'='' AND ref->>'prefix'='')
            OR (ref->>'version_id'<>'' AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(inventory) b WHERE b->>'version_id'=ref->>'version_id')) AS invalid
    )
    SELECT jsonb_build_object('status',CASE WHEN invalid THEN 'invalid'
        WHEN jsonb_array_length(items)=0 THEN 'missing' WHEN jsonb_array_length(items)=1 THEN 'resolved' ELSE 'ambiguous' END,
        'candidates',CASE WHEN invalid THEN '[]'::jsonb ELSE items END)
    FROM matches,validity;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_legacy_context_v1(project text, release_version text) RETURNS jsonb LANGUAGE plpgsql STABLE
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE inventory jsonb; matching_inventory jsonb; bindings jsonb; elements jsonb; assessments jsonb; source_refs jsonb; dependencies jsonb;
BEGIN
    IF EXISTS(SELECT 1 FROM weave_project_ontology_versions_archive l LEFT JOIN weave_ontology_body_seals b ON b.version_id=l.ontology_version_id
        WHERE l.project_id=project AND l.version_number=release_version AND b.version_id IS NULL) THEN
        RAISE EXCEPTION 'legacy context lacks a selected sealed body' USING ERRCODE='23514',CONSTRAINT='legacy_context_incomplete';
    END IF;
    IF EXISTS(SELECT 1 FROM weave_release_project_dependencies d LEFT JOIN weave_legacy_ontology_snapshots s ON s.project_id=d.source_project_id AND s.version_number=d.source_version
        WHERE d.project_id=project AND d.version_number=release_version AND s.project_id IS NULL) THEN
        RAISE EXCEPTION 'legacy context lacks a captured source' USING ERRCODE='23514',CONSTRAINT='legacy_context_incomplete';
    END IF;
    SELECT COALESCE(jsonb_agg(jsonb_build_object('ontology_id',s.ontology_id,'version_id',s.version_id,
        'digest',s.digest,'body_json',COALESCE(s.body_text,s.body::text),'selection',to_jsonb(l)) ORDER BY s.version_id COLLATE "C"),'[]')
        INTO inventory FROM weave_project_ontology_versions_archive l
        JOIN weave_ontology_body_seals s ON s.version_id=l.ontology_version_id
        WHERE l.project_id=project AND l.version_number=release_version;
    -- Matching needs only term identity and aliases. Keep the original full
    -- inventory (including exact transported body text) for the returned value.
    WITH bodies AS MATERIALIZED (
        SELECT b,(b->>'body_json')::jsonb AS body FROM jsonb_array_elements(inventory) b
    )
    SELECT COALESCE(jsonb_agg(jsonb_build_object(
        'ontology_id',b->>'ontology_id','version_id',b->>'version_id','digest',b->>'digest',
        'body_json',jsonb_build_object(
            'classes',COALESCE((SELECT jsonb_agg(jsonb_build_object(
                'uri',t->'uri','prefix',t->'prefix','local_name',t->'local_name'))
                FROM jsonb_array_elements(body->'classes') t),'[]'),
            'properties',COALESCE((SELECT jsonb_agg(jsonb_build_object(
                'uri',t->'uri','prefix',t->'prefix','local_name',t->'local_name'))
                FROM jsonb_array_elements(body->'properties') t),'[]'),
            'namespace_bindings',COALESCE(body->'namespace_bindings','[]'))::text)
        ORDER BY b->>'version_id' COLLATE "C"),'[]') INTO matching_inventory FROM bodies;
    SELECT COALESCE(jsonb_agg(to_jsonb(b) ORDER BY b.id COLLATE "C"),'[]') INTO bindings
        FROM weave_release_namespace_bindings_archive b WHERE b.release_project_id=project AND b.version_number=release_version;
    -- Retain original type/datatype/URI/QName/version values, including elements
    -- that have no reference. Normalization below never rewrites these inputs.
    SELECT COALESCE(jsonb_agg(jsonb_build_object('entity_kind',entity_kind,'entity_id',entity_id,
        'location',location,'default_kind',default_kind,'original',element)
        ORDER BY entity_kind COLLATE "C",entity_id COLLATE "C",location COLLATE "C"),'[]') INTO elements
        FROM weave_project_semantic_elements WHERE project_id=project AND version_number=release_version;
    WITH locations AS MATERIALIZED (
        SELECT entity_kind,entity_id,location,reference FROM weave_project_semantic_references
        WHERE project_id=project AND version_number=release_version
    ), distinct_references AS MATERIALIZED (
        SELECT DISTINCT reference FROM locations
    ), evaluated AS MATERIALIZED (
        SELECT reference,weave_legacy_ontology_assessment(reference,matching_inventory,bindings) AS assessment
        FROM distinct_references
    )
    SELECT COALESCE(jsonb_agg(jsonb_build_object('entity_kind',entity_kind,'entity_id',entity_id,'location',location,
        'reference',reference) || assessment
        ORDER BY entity_kind COLLATE "C",entity_id COLLATE "C",location COLLATE "C"),'[]') INTO assessments
        FROM locations JOIN evaluated USING(reference);
    SELECT COALESCE(jsonb_agg(jsonb_build_object('project_id',s.project_id,'version',s.version_number,'digest',s.context_digest)
        ORDER BY s.project_id COLLATE "C",s.version_number COLLATE "C"),'[]') INTO source_refs
        FROM weave_release_project_dependencies d JOIN weave_legacy_ontology_snapshots s
          ON s.project_id=d.source_project_id AND s.version_number=d.source_version
        WHERE d.project_id=project AND d.version_number=release_version;
    SELECT COALESCE(jsonb_agg(to_jsonb(e) ORDER BY entity_type COLLATE "C",entity_id COLLATE "C"),'[]') INTO dependencies
        FROM weave_release_dependency_entities e WHERE project_id=project AND version_number=release_version;
    RETURN jsonb_build_object('format_version',1,'project_id',project,'version',release_version,
        'inventory',inventory,'bindings',bindings,'elements',elements,'assessments',assessments,'sources','[]'::jsonb,'source_refs',source_refs,'dependencies',dependencies);
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_legacy_ontology_assessment(ref jsonb, inventory jsonb, aliases jsonb) RETURNS jsonb
LANGUAGE sql IMMUTABLE AS $$
    WITH candidates AS (
        SELECT DISTINCT b->>'ontology_id' ontology_id,b->>'version_id' version_id,b->>'digest' digest,t->>'uri' uri
        FROM jsonb_array_elements(inventory) b
        CROSS JOIN LATERAL jsonb_array_elements((b->>'body_json')::jsonb ->
            CASE ref->>'kind' WHEN 'class' THEN 'classes' WHEN 'property' THEN 'properties' ELSE '' END) t
        WHERE (ref->>'version_id'='' OR ref->>'version_id'=b->>'version_id')
          AND ((ref->>'prefix'='')=(ref->>'local_name'=''))
          AND (ref->>'uri'<>'' OR ref->>'prefix'<>'')
          AND (ref->>'uri'='' OR weave_semantic_term_matches(ref->>'uri',t,aliases || COALESCE((b->>'body_json')::jsonb->'namespace_bindings','[]')))
          AND (ref->>'prefix'='' OR weave_semantic_term_matches((ref->>'prefix') || ':' || (ref->>'local_name'),t,aliases || COALESCE((b->>'body_json')::jsonb->'namespace_bindings','[]')))
    ), matches AS (
        SELECT COALESCE(jsonb_agg(to_jsonb(c) ORDER BY version_id COLLATE "C",uri COLLATE "C"),'[]') items FROM candidates c
    ), validity AS (
        SELECT ref->>'kind' NOT IN ('class','property')
            OR ((ref->>'prefix'='')<>(ref->>'local_name'=''))
            OR (ref->>'uri'='' AND ref->>'prefix'='')
            OR (ref->>'version_id'<>'' AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(inventory) b WHERE b->>'version_id'=ref->>'version_id')) AS invalid
    )
    SELECT jsonb_build_object('status',CASE WHEN invalid THEN 'invalid'
        WHEN jsonb_array_length(items)=0 THEN 'missing' WHEN jsonb_array_length(items)=1 THEN 'resolved' ELSE 'ambiguous' END,
        'candidates',CASE WHEN invalid THEN '[]'::jsonb ELSE items END)
    FROM matches,validity;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_legacy_context_v1(project text, release_version text) RETURNS jsonb LANGUAGE plpgsql STABLE
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE inventory jsonb; bindings jsonb; elements jsonb; assessments jsonb; source_refs jsonb; dependencies jsonb;
BEGIN
    IF EXISTS(SELECT 1 FROM weave_project_ontology_versions_archive l LEFT JOIN weave_ontology_body_seals b ON b.version_id=l.ontology_version_id
        WHERE l.project_id=project AND l.version_number=release_version AND b.version_id IS NULL) THEN
        RAISE EXCEPTION 'legacy context lacks a selected sealed body' USING ERRCODE='23514',CONSTRAINT='legacy_context_incomplete';
    END IF;
    IF EXISTS(SELECT 1 FROM weave_release_project_dependencies d LEFT JOIN weave_legacy_ontology_snapshots s ON s.project_id=d.source_project_id AND s.version_number=d.source_version
        WHERE d.project_id=project AND d.version_number=release_version AND s.project_id IS NULL) THEN
        RAISE EXCEPTION 'legacy context lacks a captured source' USING ERRCODE='23514',CONSTRAINT='legacy_context_incomplete';
    END IF;
    SELECT COALESCE(jsonb_agg(jsonb_build_object('ontology_id',s.ontology_id,'version_id',s.version_id,
        'digest',s.digest,'body_json',COALESCE(s.body_text,s.body::text),'selection',to_jsonb(l)) ORDER BY s.version_id COLLATE "C"),'[]')
        INTO inventory FROM weave_project_ontology_versions_archive l
        JOIN weave_ontology_body_seals s ON s.version_id=l.ontology_version_id
        WHERE l.project_id=project AND l.version_number=release_version;
    SELECT COALESCE(jsonb_agg(to_jsonb(b) ORDER BY b.id COLLATE "C"),'[]') INTO bindings
        FROM weave_release_namespace_bindings_archive b WHERE b.release_project_id=project AND b.version_number=release_version;
    -- Retain original type/datatype/URI/QName/version values, including elements
    -- that have no reference. Normalization below never rewrites these inputs.
    SELECT COALESCE(jsonb_agg(jsonb_build_object('entity_kind',entity_kind,'entity_id',entity_id,
        'location',location,'default_kind',default_kind,'original',element)
        ORDER BY entity_kind COLLATE "C",entity_id COLLATE "C",location COLLATE "C"),'[]') INTO elements
        FROM weave_project_semantic_elements WHERE project_id=project AND version_number=release_version;
    SELECT COALESCE(jsonb_agg(jsonb_build_object('entity_kind',entity_kind,'entity_id',entity_id,'location',location,
        'reference',reference) || weave_legacy_ontology_assessment(reference,inventory,bindings)
        ORDER BY entity_kind COLLATE "C",entity_id COLLATE "C",location COLLATE "C"),'[]') INTO assessments
        FROM weave_project_semantic_references WHERE project_id=project AND version_number=release_version;
    SELECT COALESCE(jsonb_agg(jsonb_build_object('project_id',s.project_id,'version',s.version_number,'digest',s.context_digest)
        ORDER BY s.project_id COLLATE "C",s.version_number COLLATE "C"),'[]') INTO source_refs
        FROM weave_release_project_dependencies d JOIN weave_legacy_ontology_snapshots s
          ON s.project_id=d.source_project_id AND s.version_number=d.source_version
        WHERE d.project_id=project AND d.version_number=release_version;
    SELECT COALESCE(jsonb_agg(to_jsonb(e) ORDER BY entity_type COLLATE "C",entity_id COLLATE "C"),'[]') INTO dependencies
        FROM weave_release_dependency_entities e WHERE project_id=project AND version_number=release_version;
    RETURN jsonb_build_object('format_version',1,'project_id',project,'version',release_version,
        'inventory',inventory,'bindings',bindings,'elements',elements,'assessments',assessments,'sources','[]'::jsonb,'source_refs',source_refs,'dependencies',dependencies);
END $$;
-- +goose StatementEnd
