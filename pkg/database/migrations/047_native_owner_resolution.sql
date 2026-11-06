-- +goose Up
-- One term, one owner. An extension may redeclare a parent class under the
-- parent's URI (Linked Art 0.9 carries six CIDOC CRM classes). The Go
-- resolvers (ontologyrelease, ontologyevidence) now treat every match that
-- shares one URI as one term owned by the ontology declaring it under its own
-- prefix. Legacy evidence is re-assessed on every read and must reproduce the
-- captured assessment exactly, so the storage twins follow the same rule:
--   * weave_legacy_matching_inventory / weave_legacy_context_v1 keep each
--     body's ontology prefix in the matching inventory;
--   * weave_legacy_ontology_assessment prefers the native owner;
--   * weave_project_semantic_candidates (the capture guard) collapses identical
--     URIs to the native owner.
-- Captured rows are not rewritten: evidence captured before this migration
-- under the old rule stays as it was and is re-captured by the next rebuild.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_legacy_matching_inventory(inventory jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
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
            'namespace_bindings',COALESCE(body->'namespace_bindings','[]'),
            'ontology',jsonb_build_object('prefix',COALESCE(body->'ontology'->>'prefix','')))::text)
        ORDER BY b->>'version_id' COLLATE "C"),'[]') FROM bodies;
$$;
-- +goose StatementEnd
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
        -- native: the owner declares the term under its own prefix. An extension
        -- may redeclare a parent's term under the parent's URI; the same URI is
        -- the same term, owned by the ontology whose prefix it carries.
        SELECT b.ontology_id,b.version_id,b.digest,t->>'uri' uri,
            bool_or(COALESCE(t->>'prefix','')<>'' AND t->>'prefix'=b.body->'ontology'->>'prefix') AS native
        FROM bodies b
        CROSS JOIN LATERAL jsonb_array_elements(b.body ->
            CASE ref->>'kind' WHEN 'class' THEN 'classes' WHEN 'property' THEN 'properties' ELSE '' END) t
        WHERE (ref->>'version_id'='' OR ref->>'version_id'=b.version_id)
          AND ((ref->>'prefix'='')=(ref->>'local_name'=''))
          AND (ref->>'uri'<>'' OR ref->>'prefix'<>'')
          AND (ref->>'uri'='' OR weave_semantic_term_matches(ref->>'uri',t,aliases || COALESCE(b.body->'namespace_bindings','[]')))
          AND (ref->>'prefix'='' OR weave_semantic_term_matches((ref->>'prefix') || ':' || (ref->>'local_name'),t,aliases || COALESCE(b.body->'namespace_bindings','[]')))
        GROUP BY b.ontology_id,b.version_id,b.digest,t->>'uri'
    ), chosen AS (
        -- When every candidate shares one URI and exactly one owner is native,
        -- that owner is the identity; foreign copies alone, or differing URIs,
        -- stay ambiguous.
        SELECT c.ontology_id,c.version_id,c.digest,c.uri FROM candidates c
        WHERE c.native OR NOT (
            (SELECT count(*) FROM candidates)>1
            AND (SELECT count(DISTINCT uri) FROM candidates)=1
            AND (SELECT count(*) FROM candidates WHERE native)=1)
    ), matches AS (
        SELECT COALESCE(jsonb_agg(to_jsonb(c) ORDER BY version_id COLLATE "C",uri COLLATE "C"),'[]') items FROM chosen c
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
            'namespace_bindings',COALESCE(body->'namespace_bindings','[]'),
            'ontology',jsonb_build_object('prefix',COALESCE(body->'ontology'->>'prefix','')))::text)
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
CREATE OR REPLACE VIEW weave_project_semantic_candidates AS
WITH bodies AS (
    SELECT p.project_id,p.version_number,p.ontology_id,p.ontology_version_id,p.digest,
        (o.artifact::jsonb->>'body_json')::jsonb AS body
    FROM weave_release_ontology_dependencies p
    JOIN weave_ontology_releases o ON (o.ontology_id,o.version_id,o.digest)=(p.ontology_id,p.ontology_version_id,p.digest)
), bindings AS (
    SELECT release_project_id AS project_id,version_number,
        jsonb_agg(jsonb_build_object('prefix',prefix,'namespace',namespace)) AS aliases
    FROM weave_release_namespace_bindings_archive GROUP BY release_project_id,version_number
), terms AS (
    SELECT b.project_id,b.version_number,b.ontology_id,b.ontology_version_id,b.digest,'class'::text AS kind,
        term->>'uri' AS uri,term->>'prefix' AS prefix,term->>'local_name' AS local_name,
        COALESCE(term->>'prefix','')<>'' AND term->>'prefix'=b.body->'ontology'->>'prefix' AS native
    FROM bodies b CROSS JOIN LATERAL jsonb_array_elements(COALESCE(b.body->'classes','[]'::jsonb)) term
    WHERE COALESCE(term->>'uri','')<>''
    UNION ALL
    SELECT b.project_id,b.version_number,b.ontology_id,b.ontology_version_id,b.digest,'property'::text,
        term->>'uri',term->>'prefix',term->>'local_name',
        COALESCE(term->>'prefix','')<>'' AND term->>'prefix'=b.body->'ontology'->>'prefix'
    FROM bodies b CROSS JOIN LATERAL jsonb_array_elements(COALESCE(b.body->'properties','[]'::jsonb)) term
    WHERE COALESCE(term->>'uri','')<>''
), refs AS (
    -- One row per string the reference must match: its URI, its QName, or both.
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,
        r.reference->>'kind' AS kind,r.reference->>'version_id' AS version_id,r.reference->>'uri' AS ref,
        (CASE WHEN r.reference->>'uri'<>'' THEN 1 ELSE 0 END)+(CASE WHEN r.reference->>'prefix'<>'' THEN 1 ELSE 0 END) AS required
    FROM weave_project_semantic_references r
    WHERE ((r.reference->>'prefix'='')=(r.reference->>'local_name'='')) AND r.reference->>'uri'<>''
    UNION ALL
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,
        r.reference->>'kind',r.reference->>'version_id',(r.reference->>'prefix')||':'||(r.reference->>'local_name'),
        (CASE WHEN r.reference->>'uri'<>'' THEN 1 ELSE 0 END)+(CASE WHEN r.reference->>'prefix'<>'' THEN 1 ELSE 0 END)
    FROM weave_project_semantic_references r
    WHERE ((r.reference->>'prefix'='')=(r.reference->>'local_name'='')) AND r.reference->>'prefix'<>''
), keys AS (
    -- Storage counterpart of namespace.MatchesTerm, expanded ahead of the join:
    -- the exact URI, the term's own QName, and every declared alias expansion.
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,r.kind,r.ref,r.required,
        b.ontology_id,b.ontology_version_id,b.digest,
        r.ref AS key_uri,NULL::text AS key_prefix,NULL::text AS key_local
    FROM refs r JOIN bodies b USING (project_id,version_number)
    WHERE r.version_id='' OR r.version_id=b.ontology_version_id
    UNION ALL
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,r.kind,r.ref,r.required,
        b.ontology_id,b.ontology_version_id,b.digest,
        NULL::text,split_part(r.ref,':',1),substring(r.ref FROM strpos(r.ref,':')+1)
    FROM refs r JOIN bodies b USING (project_id,version_number)
    WHERE (r.version_id='' OR r.version_id=b.ontology_version_id)
      AND strpos(r.ref,':')>1 AND substring(r.ref FROM strpos(r.ref,':')+1)<>''
      AND substring(r.ref FROM strpos(r.ref,':')+1 FOR 2)<>'//'
    UNION ALL
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,r.kind,r.ref,r.required,
        b.ontology_id,b.ontology_version_id,b.digest,
        (al->>'namespace')||substring(r.ref FROM strpos(r.ref,':')+1),NULL::text,NULL::text
    FROM refs r JOIN bodies b USING (project_id,version_number)
    LEFT JOIN bindings a USING (project_id,version_number)
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(a.aliases,'[]'::jsonb)||COALESCE(b.body->'namespace_bindings','[]'::jsonb)) al
    WHERE (r.version_id='' OR r.version_id=b.ontology_version_id)
      AND strpos(r.ref,':')>1 AND substring(r.ref FROM strpos(r.ref,':')+1)<>''
      AND substring(r.ref FROM strpos(r.ref,':')+1 FOR 2)<>'//'
      AND al->>'prefix'=split_part(r.ref,':',1)
), matched AS (
    SELECT DISTINCT project_id,version_number,entity_kind,entity_id,location,ref,required,
        ontology_id,ontology_version_id,digest,uri,native
    FROM (
        SELECT k.project_id,k.version_number,k.entity_kind,k.entity_id,k.location,k.ref,k.required,
            k.ontology_id,k.ontology_version_id,k.digest,t.uri,t.native
        FROM keys k
        JOIN terms t ON (t.project_id,t.version_number,t.ontology_id,t.ontology_version_id,t.digest,t.kind)
            =(k.project_id,k.version_number,k.ontology_id,k.ontology_version_id,k.digest,k.kind)
            AND t.uri=k.key_uri
        WHERE k.key_uri IS NOT NULL
        UNION ALL
        SELECT k.project_id,k.version_number,k.entity_kind,k.entity_id,k.location,k.ref,k.required,
            k.ontology_id,k.ontology_version_id,k.digest,t.uri,t.native
        FROM keys k
        JOIN terms t ON (t.project_id,t.version_number,t.ontology_id,t.ontology_version_id,t.digest,t.kind)
            =(k.project_id,k.version_number,k.ontology_id,k.ontology_version_id,k.digest,k.kind)
            AND t.prefix=k.key_prefix AND t.local_name=k.key_local
        WHERE k.key_prefix IS NOT NULL
    ) u
)
, candidates AS (
    -- A term is a candidate only when it matches every string the reference carries.
    SELECT project_id,version_number,entity_kind,entity_id,location,ontology_id,ontology_version_id,digest,uri,
        bool_or(native) AS native
    FROM matched
    GROUP BY project_id,version_number,entity_kind,entity_id,location,ontology_id,ontology_version_id,digest,uri,required
    HAVING count(*)=required
), scored AS (
    SELECT c.*,
        count(*) OVER w AS n,
        (min(uri) OVER w)=(max(uri) OVER w) AS one_uri,
        sum(CASE WHEN native THEN 1 ELSE 0 END) OVER w AS natives
    FROM candidates c
    WINDOW w AS (PARTITION BY project_id,version_number,entity_kind,entity_id,location)
)
-- An extension may redeclare a parent's term under the parent's URI: when every
-- candidate at a location shares one URI and exactly one owner declares it under
-- its own prefix, that owner is the identity (same rule as the Go resolvers).
SELECT project_id,version_number,entity_kind,entity_id,location,ontology_id,ontology_version_id,digest,uri
FROM scored
WHERE native OR NOT (n>1 AND one_uri AND natives=1);

-- +goose Down
-- Restore the migration 038/039 functions and the migration 045 view.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_legacy_matching_inventory(inventory jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
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
        ORDER BY b->>'version_id' COLLATE "C"),'[]') FROM bodies;
$$;
-- +goose StatementEnd
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
CREATE OR REPLACE VIEW weave_project_semantic_candidates AS
WITH bodies AS (
    SELECT p.project_id,p.version_number,p.ontology_id,p.ontology_version_id,p.digest,
        (o.artifact::jsonb->>'body_json')::jsonb AS body
    FROM weave_release_ontology_dependencies p
    JOIN weave_ontology_releases o ON (o.ontology_id,o.version_id,o.digest)=(p.ontology_id,p.ontology_version_id,p.digest)
), bindings AS (
    SELECT release_project_id AS project_id,version_number,
        jsonb_agg(jsonb_build_object('prefix',prefix,'namespace',namespace)) AS aliases
    FROM weave_release_namespace_bindings_archive GROUP BY release_project_id,version_number
), terms AS (
    SELECT b.project_id,b.version_number,b.ontology_id,b.ontology_version_id,b.digest,'class'::text AS kind,
        term->>'uri' AS uri,term->>'prefix' AS prefix,term->>'local_name' AS local_name
    FROM bodies b CROSS JOIN LATERAL jsonb_array_elements(COALESCE(b.body->'classes','[]'::jsonb)) term
    WHERE COALESCE(term->>'uri','')<>''
    UNION ALL
    SELECT b.project_id,b.version_number,b.ontology_id,b.ontology_version_id,b.digest,'property'::text,
        term->>'uri',term->>'prefix',term->>'local_name'
    FROM bodies b CROSS JOIN LATERAL jsonb_array_elements(COALESCE(b.body->'properties','[]'::jsonb)) term
    WHERE COALESCE(term->>'uri','')<>''
), refs AS (
    -- One row per string the reference must match: its URI, its QName, or both.
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,
        r.reference->>'kind' AS kind,r.reference->>'version_id' AS version_id,r.reference->>'uri' AS ref,
        (CASE WHEN r.reference->>'uri'<>'' THEN 1 ELSE 0 END)+(CASE WHEN r.reference->>'prefix'<>'' THEN 1 ELSE 0 END) AS required
    FROM weave_project_semantic_references r
    WHERE ((r.reference->>'prefix'='')=(r.reference->>'local_name'='')) AND r.reference->>'uri'<>''
    UNION ALL
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,
        r.reference->>'kind',r.reference->>'version_id',(r.reference->>'prefix')||':'||(r.reference->>'local_name'),
        (CASE WHEN r.reference->>'uri'<>'' THEN 1 ELSE 0 END)+(CASE WHEN r.reference->>'prefix'<>'' THEN 1 ELSE 0 END)
    FROM weave_project_semantic_references r
    WHERE ((r.reference->>'prefix'='')=(r.reference->>'local_name'='')) AND r.reference->>'prefix'<>''
), keys AS (
    -- Storage counterpart of namespace.MatchesTerm, expanded ahead of the join:
    -- the exact URI, the term's own QName, and every declared alias expansion.
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,r.kind,r.ref,r.required,
        b.ontology_id,b.ontology_version_id,b.digest,
        r.ref AS key_uri,NULL::text AS key_prefix,NULL::text AS key_local
    FROM refs r JOIN bodies b USING (project_id,version_number)
    WHERE r.version_id='' OR r.version_id=b.ontology_version_id
    UNION ALL
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,r.kind,r.ref,r.required,
        b.ontology_id,b.ontology_version_id,b.digest,
        NULL::text,split_part(r.ref,':',1),substring(r.ref FROM strpos(r.ref,':')+1)
    FROM refs r JOIN bodies b USING (project_id,version_number)
    WHERE (r.version_id='' OR r.version_id=b.ontology_version_id)
      AND strpos(r.ref,':')>1 AND substring(r.ref FROM strpos(r.ref,':')+1)<>''
      AND substring(r.ref FROM strpos(r.ref,':')+1 FOR 2)<>'//'
    UNION ALL
    SELECT r.project_id,r.version_number,r.entity_kind,r.entity_id,r.location,r.kind,r.ref,r.required,
        b.ontology_id,b.ontology_version_id,b.digest,
        (al->>'namespace')||substring(r.ref FROM strpos(r.ref,':')+1),NULL::text,NULL::text
    FROM refs r JOIN bodies b USING (project_id,version_number)
    LEFT JOIN bindings a USING (project_id,version_number)
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(a.aliases,'[]'::jsonb)||COALESCE(b.body->'namespace_bindings','[]'::jsonb)) al
    WHERE (r.version_id='' OR r.version_id=b.ontology_version_id)
      AND strpos(r.ref,':')>1 AND substring(r.ref FROM strpos(r.ref,':')+1)<>''
      AND substring(r.ref FROM strpos(r.ref,':')+1 FOR 2)<>'//'
      AND al->>'prefix'=split_part(r.ref,':',1)
), matched AS (
    SELECT DISTINCT project_id,version_number,entity_kind,entity_id,location,ref,required,
        ontology_id,ontology_version_id,digest,uri
    FROM (
        SELECT k.project_id,k.version_number,k.entity_kind,k.entity_id,k.location,k.ref,k.required,
            k.ontology_id,k.ontology_version_id,k.digest,t.uri
        FROM keys k
        JOIN terms t ON (t.project_id,t.version_number,t.ontology_id,t.ontology_version_id,t.digest,t.kind)
            =(k.project_id,k.version_number,k.ontology_id,k.ontology_version_id,k.digest,k.kind)
            AND t.uri=k.key_uri
        WHERE k.key_uri IS NOT NULL
        UNION ALL
        SELECT k.project_id,k.version_number,k.entity_kind,k.entity_id,k.location,k.ref,k.required,
            k.ontology_id,k.ontology_version_id,k.digest,t.uri
        FROM keys k
        JOIN terms t ON (t.project_id,t.version_number,t.ontology_id,t.ontology_version_id,t.digest,t.kind)
            =(k.project_id,k.version_number,k.ontology_id,k.ontology_version_id,k.digest,k.kind)
            AND t.prefix=k.key_prefix AND t.local_name=k.key_local
        WHERE k.key_prefix IS NOT NULL
    ) u
)
-- A term is a candidate only when it matches every string the reference carries.
SELECT project_id,version_number,entity_kind,entity_id,location,ontology_id,ontology_version_id,digest,uri
FROM matched
GROUP BY project_id,version_number,entity_kind,entity_id,location,ontology_id,ontology_version_id,digest,uri,required
HAVING count(*)=required;
