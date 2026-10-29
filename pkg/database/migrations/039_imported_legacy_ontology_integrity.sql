-- +goose Up
-- Extract migration 030's normalization so archive capture and imported evidence
-- cannot disagree about original element locations, additional types or datatype.
-- +goose StatementBegin
CREATE FUNCTION weave_semantic_element_references(element jsonb, default_kind text, base_location text)
RETURNS TABLE(location text, reference jsonb) LANGUAGE sql IMMUTABLE AS $$
    WITH refs AS (
        SELECT base_location AS location,element,
            CASE lower(COALESCE(NULLIF(element->>'type',''),default_kind))
                WHEN 'literal' THEN 'class' ELSE lower(COALESCE(NULLIF(element->>'type',''),default_kind)) END AS kind
        WHERE element IS NOT NULL AND element <> 'null'::jsonb AND element <> '{}'::jsonb
          AND lower(COALESCE(element->>'type','')) <> 'complete'
          AND (COALESCE(element->>'type','') <> '' OR COALESCE(element->>'uri','') <> '' OR COALESCE(element->>'prefix','') <> '' OR COALESCE(element->>'local_name','') <> '' OR COALESCE(element->>'ontology_version_id','') <> '')
        UNION ALL SELECT base_location || '/additional_types/' || (a.n-1),a.element,'class'
        FROM jsonb_array_elements(NULLIF(element->'additional_types','null'::jsonb)) WITH ORDINALITY a(element,n)
        UNION ALL SELECT base_location || '/datatype',jsonb_build_object('uri',element->>'datatype'),'class'
        WHERE COALESCE(element->>'datatype','') <> ''
    )
    SELECT location,jsonb_build_object('kind',kind,'uri',COALESCE(element->>'uri',''),
        'prefix',COALESCE(element->>'prefix',''),'local_name',COALESCE(element->>'local_name',''),
        'version_id',COALESCE(element->>'ontology_version_id','')) FROM refs;
$$;
-- +goose StatementEnd

CREATE OR REPLACE VIEW weave_project_semantic_references AS
SELECT e.project_id,e.version_number,e.entity_kind,e.entity_id,r.location::text AS location,r.reference::jsonb AS reference
FROM weave_project_semantic_elements e
CROSS JOIN LATERAL weave_semantic_element_references(e.element,e.default_kind,e.location) AS r(location,reference);

-- Share the exact compact matching projection from migration 038. Full body_json
-- strings remain exclusively in retained evidence; no RDF is parsed or rewritten.
-- +goose StatementBegin
CREATE FUNCTION weave_legacy_matching_inventory(inventory jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
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
    matching_inventory := weave_legacy_matching_inventory(inventory);
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

-- +goose StatementBegin
CREATE FUNCTION weave_imported_legacy_local_context(doc jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
    SELECT (doc - 'sources') || jsonb_object_agg(k,COALESCE(NULLIF(doc->k,'null'),'[]'::jsonb))
    FROM unnest(ARRAY['inventory','bindings','elements','assessments','source_refs','dependencies']) k;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION weave_lock_imported_legacy_contexts(payloads text[]) RETURNS void LANGUAGE plpgsql
SET search_path=public,pg_temp AS $$
DECLARE origin record;
BEGIN
    -- An API bundle locks every origin before body writes. The row trigger uses
    -- the same sorted ordering for raw single-document inserts.
    FOR origin IN
        WITH docs AS MATERIALIZED (SELECT payload::jsonb doc FROM unnest(payloads) payload),
        contexts AS (SELECT doc FROM docs UNION ALL
            SELECT (s->>'json')::jsonb FROM docs,LATERAL jsonb_array_elements(COALESCE(NULLIF(doc->'sources','null'),'[]')) s)
        SELECT project,version FROM (
            SELECT DISTINCT doc->>'project_id' AS project,doc->>'version' AS version FROM contexts
        ) origins
        ORDER BY project COLLATE "C",version COLLATE "C"
    LOOP
        PERFORM pg_advisory_xact_lock(hashtextextended('imported-legacy-context:' || jsonb_build_array(origin.project,origin.version)::text,0));
    END LOOP;
END $$;
-- +goose StatementEnd

CREATE TABLE weave_imported_legacy_ontology_contexts (
    project_id text NOT NULL CHECK (btrim(project_id)<>''),
    version_number text NOT NULL CHECK (btrim(version_number)<>''),
    context_document jsonb NOT NULL,
    retained_project_id text NOT NULL,
    retained_version text NOT NULL,
    PRIMARY KEY (project_id,version_number),
    FOREIGN KEY (retained_project_id,retained_version)
        REFERENCES weave_imported_legacy_ontology_snapshots(project_id,version_number) ON DELETE RESTRICT
);

-- Validate the original local inputs once for a new context. The expensive term
-- projection is built once; identical normalized references share an assessment.
-- This is evidence validation, not publication or unique semantic resolution.
-- +goose StatementBegin
CREATE FUNCTION weave_validate_imported_legacy_context(doc jsonb) RETURNS void LANGUAGE plpgsql
SET search_path=public,pg_temp AS $$
DECLARE item jsonb; original jsonb; previous text := ''; compact jsonb; expected jsonb; actual jsonb;
BEGIN
    FOR item IN SELECT value FROM jsonb_array_elements(doc->'inventory') LOOP
        IF COALESCE(item->>'version_id','') COLLATE "C" <= previous COLLATE "C"
           OR COALESCE(item->>'ontology_id','')=''
           OR jsonb_typeof(item->'selection') IS DISTINCT FROM 'object'
           OR EXISTS(SELECT 1 FROM jsonb_each(item->'selection') f WHERE f.key IN ('project_id','version_number','ontology_version_id') AND jsonb_typeof(f.value) NOT IN ('string','null'))
           OR EXISTS(SELECT 1 FROM jsonb_each(item) f WHERE f.key IN ('ontology_id','version_id','body_json','digest') AND jsonb_typeof(f.value) IS DISTINCT FROM 'string')
           OR (item#>>'{selection,project_id}') IS DISTINCT FROM doc->>'project_id'
           OR (item#>>'{selection,version_number}') IS DISTINCT FROM doc->>'version'
           OR (item#>>'{selection,ontology_version_id}') IS DISTINCT FROM item->>'version_id'
           OR NOT EXISTS (SELECT 1 FROM weave_ontology_body_seals s
               WHERE s.version_id=item->>'version_id' AND s.ontology_id=item->>'ontology_id'
                 AND s.digest=item->>'digest' AND COALESCE(s.body_text,s.body::text)=item->>'body_json') THEN
            RAISE EXCEPTION 'invalid imported context body inventory' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
        END IF;
        previous := item->>'version_id';
    END LOOP;
    IF EXISTS (SELECT 1 FROM jsonb_array_elements(doc->'elements') e
        GROUP BY COALESCE(e->>'entity_kind',''),COALESCE(e->>'entity_id',''),COALESCE(e->>'location','') HAVING count(*)>1) THEN
        RAISE EXCEPTION 'duplicate imported semantic element' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
    END IF;
    FOR item IN SELECT value FROM jsonb_array_elements(doc->'elements') LOOP
        IF jsonb_typeof(item) IS DISTINCT FROM 'object' OR NOT item ? 'original'
           OR EXISTS (SELECT 1 FROM jsonb_each(item) f WHERE f.key IN ('entity_kind','entity_id','location','default_kind')
               AND jsonb_typeof(f.value) NOT IN ('string','null'))
           OR jsonb_typeof(item->'original') NOT IN ('object','null') THEN
            RAISE EXCEPTION 'invalid imported semantic element shape' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
        END IF;
        original := item->'original';
        IF jsonb_typeof(COALESCE(NULLIF(original->'additional_types','null'),'[]')) <> 'array' THEN
            RAISE EXCEPTION 'invalid imported additional types' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
        END IF;
        -- Go decodes these same known fields as strings (or null). Additional
        -- types are normalized only one level deep, exactly as in migration 030.
        IF EXISTS (
            SELECT 1 FROM (
                SELECT original AS value UNION ALL
                SELECT value FROM jsonb_array_elements(COALESCE(NULLIF(original->'additional_types','null'),'[]'))
            ) originals WHERE jsonb_typeof(value) NOT IN ('object','null')
        ) THEN
            RAISE EXCEPTION 'invalid imported original reference shape' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
        END IF;
        IF EXISTS (
            SELECT 1 FROM (
                SELECT original AS value UNION ALL
                SELECT value FROM jsonb_array_elements(COALESCE(NULLIF(original->'additional_types','null'),'[]'))
            ) originals, LATERAL jsonb_each(COALESCE(NULLIF(originals.value,'null'),'{}')) f
            WHERE (f.key IN ('type','uri','prefix','local_name','ontology_version_id','datatype') AND jsonb_typeof(f.value) NOT IN ('string','null'))
               OR (f.key='additional_types' AND jsonb_typeof(f.value) NOT IN ('array','null'))
        ) THEN
            RAISE EXCEPTION 'invalid imported original reference fields' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
        END IF;
    END LOOP;
    IF EXISTS (SELECT 1 FROM jsonb_array_elements(doc->'bindings') b
        WHERE jsonb_typeof(b) IS DISTINCT FROM 'object'
           OR EXISTS(SELECT 1 FROM jsonb_each(b) f WHERE f.key IN ('prefix','namespace','source') AND jsonb_typeof(f.value) NOT IN ('string','null'))
           OR (b ? 'weight' AND b->'weight'<>'null' AND (jsonb_typeof(b->'weight')<>'number' OR b->>'weight' !~ '^-?[0-9]+$'))) THEN
        RAISE EXCEPTION 'invalid imported namespace binding' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
    END IF;
    -- Enforce int64 bounds, matching namespace.Binding's decoder.
    PERFORM (b->>'weight')::bigint FROM jsonb_array_elements(doc->'bindings') b;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements(doc->'elements') e,
        LATERAL weave_semantic_element_references(e->'original',COALESCE(e->>'default_kind',''),COALESCE(e->>'location','')) r
        GROUP BY COALESCE(e->>'entity_kind',''),COALESCE(e->>'entity_id',''),r.location HAVING count(*)>1
    ) THEN
        RAISE EXCEPTION 'duplicate imported semantic location' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
    END IF;
    compact := weave_legacy_matching_inventory(doc->'inventory');
    WITH locations AS MATERIALIZED (
        SELECT COALESCE(e->>'entity_kind','') entity_kind,COALESCE(e->>'entity_id','') entity_id,r.location,r.reference
        FROM jsonb_array_elements(doc->'elements') e,
        LATERAL weave_semantic_element_references(e->'original',COALESCE(e->>'default_kind',''),COALESCE(e->>'location','')) r
    ), refs AS MATERIALIZED (SELECT DISTINCT reference FROM locations),
    assessed AS MATERIALIZED (
        SELECT reference,weave_legacy_ontology_assessment(reference,compact,doc->'bindings') assessment FROM refs
    )
    SELECT COALESCE(jsonb_agg(jsonb_build_object('entity_kind',entity_kind,'entity_id',entity_id,'location',location,
        'reference',reference) || assessment ORDER BY entity_kind COLLATE "C",entity_id COLLATE "C",location COLLATE "C"),'[]')
    INTO expected FROM locations JOIN assessed USING(reference);
    SELECT COALESCE(jsonb_agg(a ORDER BY a->>'entity_kind' COLLATE "C",a->>'entity_id' COLLATE "C",a->>'location' COLLATE "C"),'[]')
        INTO actual FROM jsonb_array_elements(doc->'assessments') a;
    IF expected IS DISTINCT FROM actual THEN
        RAISE EXCEPTION 'imported assessments differ from original reference coverage or candidates' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
    END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION weave_validate_imported_legacy_graph(doc jsonb) RETURNS jsonb LANGUAGE plpgsql
SET search_path=public,pg_temp AS $$
DECLARE contexts jsonb := jsonb_build_array(weave_imported_legacy_local_context(doc)); source_index jsonb := '{}';
    source jsonb; local_doc jsonb; ref jsonb; cycle boolean; reached bigint;
BEGIN
    IF jsonb_typeof(COALESCE(NULLIF(doc->'sources','null'),'[]')) <> 'array' THEN
        RAISE EXCEPTION 'invalid imported sources' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
    END IF;
    FOR source IN SELECT value FROM jsonb_array_elements(COALESCE(NULLIF(doc->'sources','null'),'[]')) LOOP
        IF jsonb_typeof(source->'json') IS DISTINCT FROM 'string' OR jsonb_typeof(source->'digest') IS DISTINCT FROM 'string'
           OR encode(sha256(convert_to(source->>'json','UTF8')),'hex') IS DISTINCT FROM source->>'digest' THEN
            RAISE EXCEPTION 'invalid imported source digest' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
        END IF;
        local_doc := (source->>'json')::jsonb;
        IF COALESCE(NULLIF(local_doc->'sources','null'),'[]') <> '[]'::jsonb THEN
            RAISE EXCEPTION 'nested imported source context' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
        END IF;
        source_index := source_index || jsonb_build_object(jsonb_build_array(local_doc->>'project_id',local_doc->>'version')::text,source->'digest');
        contexts := contexts || jsonb_build_array(weave_imported_legacy_local_context(local_doc));
    END LOOP;
    IF EXISTS (SELECT 1 FROM jsonb_array_elements(contexts) c GROUP BY c->>'project_id',c->>'version' HAVING count(*)>1) THEN
        RAISE EXCEPTION 'duplicate imported source origin' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
    END IF;
    FOR local_doc IN SELECT value FROM jsonb_array_elements(contexts) LOOP
        IF jsonb_typeof(local_doc->'format_version') IS DISTINCT FROM 'number' OR (local_doc->>'format_version') IS DISTINCT FROM '1'
           OR jsonb_typeof(local_doc->'project_id') IS DISTINCT FROM 'string' OR COALESCE(btrim(local_doc->>'project_id'),'')=''
           OR jsonb_typeof(local_doc->'version') IS DISTINCT FROM 'string' OR COALESCE(btrim(local_doc->>'version'),'')=''
           OR EXISTS (SELECT 1 FROM unnest(ARRAY['inventory','bindings','elements','assessments','source_refs','dependencies']) k
               WHERE jsonb_typeof(local_doc->k) IS DISTINCT FROM 'array') THEN
            RAISE EXCEPTION 'invalid imported local context shape' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
        END IF;
        IF EXISTS (SELECT 1 FROM jsonb_array_elements(local_doc->'source_refs') r
            GROUP BY r->>'project_id',r->>'version' HAVING count(*)>1) THEN
            RAISE EXCEPTION 'duplicate imported source reference' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
        END IF;
        FOR ref IN SELECT value FROM jsonb_array_elements(local_doc->'source_refs') LOOP
            IF jsonb_typeof(ref) IS DISTINCT FROM 'object'
               OR EXISTS(SELECT 1 FROM jsonb_each(ref) f WHERE f.key IN ('project_id','version','digest') AND jsonb_typeof(f.value) NOT IN ('string','null'))
               OR COALESCE(ref->>'digest','')=''
               OR source_index->>jsonb_build_array(ref->>'project_id',ref->>'version')::text IS DISTINCT FROM ref->>'digest' THEN
                RAISE EXCEPTION 'missing or mismatched imported source reference' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
            END IF;
        END LOOP;
        IF EXISTS (SELECT 1 FROM jsonb_array_elements(local_doc->'dependencies') d
            WHERE jsonb_typeof(d) IS DISTINCT FROM 'object'
               OR EXISTS(SELECT 1 FROM jsonb_each(d) f WHERE f.key IN ('project_id','version_number','entity_type','entity_id','source_project_id','source_version') AND jsonb_typeof(f.value) NOT IN ('string','null'))
               OR d->'project_id' IS DISTINCT FROM local_doc->'project_id' OR d->'version_number' IS DISTINCT FROM local_doc->'version'
               OR NOT EXISTS (SELECT 1 FROM jsonb_array_elements(local_doc->'source_refs') r
                   WHERE r->'project_id'=d->'source_project_id' AND r->'version'=d->'source_version')
               OR EXISTS (SELECT 1 FROM jsonb_array_elements(local_doc->'elements') e
                   WHERE COALESCE(e->>'entity_kind','')=COALESCE(d->>'entity_type','') AND COALESCE(e->>'entity_id','')=COALESCE(d->>'entity_id','')))
           OR EXISTS (SELECT 1 FROM jsonb_array_elements(local_doc->'dependencies') d GROUP BY COALESCE(d->>'entity_type',''),COALESCE(d->>'entity_id','') HAVING count(*)>1) THEN
            RAISE EXCEPTION 'imported entity dependency escapes its source context' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
        END IF;
    END LOOP;
    -- UNION, not UNION ALL: transitive closure stays bounded by origin pairs,
    -- including dense DAGs. It detects cycles without enumerating every path.
    WITH RECURSIVE edges AS (
        SELECT jsonb_build_array(c->>'project_id',c->>'version') parent,jsonb_build_array(r->>'project_id',r->>'version') child
        FROM jsonb_array_elements(contexts) c,LATERAL jsonb_array_elements(c->'source_refs') r
    ), reach(parent,child) AS (
        SELECT parent,child FROM edges UNION
        SELECT r.parent,e.child FROM reach r JOIN edges e ON r.child=e.parent
    )
    SELECT COALESCE(bool_or(parent=child),false),count(DISTINCT child) FILTER (
        WHERE parent=jsonb_build_array(doc->>'project_id',doc->>'version')) INTO cycle,reached FROM reach;
    IF cycle OR reached+1 <> jsonb_array_length(contexts) THEN
        RAISE EXCEPTION 'cyclic or unreferenced imported source context' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
    END IF;
    RETURN contexts;
END $$;
-- +goose StatementEnd

-- The ledger can only retain a context actually embedded in an immutable import.
-- An insert guard also validates it: raw ledger inserts cannot manufacture a
-- trusted row that would make future imports skip validation.
-- +goose StatementBegin
CREATE FUNCTION weave_guard_imported_legacy_context_insert() RETURNS trigger LANGUAGE plpgsql
SET search_path=public,pg_temp AS $$
DECLARE doc jsonb;
BEGIN
    SELECT document::jsonb INTO doc FROM weave_imported_legacy_ontology_snapshots
        WHERE project_id=NEW.retained_project_id AND version_number=NEW.retained_version;
    IF doc IS NULL OR (NEW.context_document->>'project_id') IS DISTINCT FROM NEW.project_id
       OR (NEW.context_document->>'version') IS DISTINCT FROM NEW.version_number
       OR NOT EXISTS (SELECT 1 FROM (
           SELECT weave_imported_legacy_local_context(doc) c UNION ALL
           SELECT weave_imported_legacy_local_context((s->>'json')::jsonb)
           FROM jsonb_array_elements(COALESCE(NULLIF(doc->'sources','null'),'[]')) s
       ) contexts WHERE c=NEW.context_document) THEN
        RAISE EXCEPTION 'context ledger lacks matching imported evidence' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
    END IF;
    PERFORM weave_validate_imported_legacy_context(NEW.context_document);
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER validate_imported_legacy_context BEFORE INSERT ON weave_imported_legacy_ontology_contexts
FOR EACH ROW EXECUTE FUNCTION weave_guard_imported_legacy_context_insert();

-- +goose StatementBegin
CREATE FUNCTION weave_guard_imported_legacy_context_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP<>'TRUNCATE' OR EXISTS(SELECT 1 FROM weave_imported_legacy_ontology_contexts) THEN
        RAISE EXCEPTION 'imported legacy context ledger is retained and immutable' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
    END IF;
    RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_imported_legacy_context BEFORE UPDATE OR DELETE ON weave_imported_legacy_ontology_contexts
FOR EACH ROW EXECUTE FUNCTION weave_guard_imported_legacy_context_change();
CREATE TRIGGER guard_imported_legacy_context_truncate BEFORE TRUNCATE ON weave_imported_legacy_ontology_contexts
FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_imported_legacy_context_change();

-- +goose StatementBegin
CREATE FUNCTION weave_record_imported_legacy_contexts(payload text) RETURNS void LANGUAGE plpgsql
SET search_path=public,pg_temp AS $$
DECLARE doc jsonb := payload::jsonb; contexts jsonb; local_doc jsonb; existing jsonb;
BEGIN
    contexts := weave_validate_imported_legacy_graph(doc);
    PERFORM weave_lock_imported_legacy_contexts(ARRAY[payload]);
    FOR local_doc IN SELECT value FROM jsonb_array_elements(contexts)
        ORDER BY value->>'project_id' COLLATE "C",value->>'version' COLLATE "C"
    LOOP
        SELECT context_document INTO existing FROM weave_imported_legacy_ontology_contexts
            WHERE project_id=local_doc->>'project_id' AND version_number=local_doc->>'version';
        IF FOUND THEN
            IF existing IS DISTINCT FROM local_doc THEN
                RAISE EXCEPTION 'persisted imported source context conflicts' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
            END IF;
            CONTINUE; -- already validated; do not project bodies or assess again
        END IF;
        INSERT INTO weave_imported_legacy_ontology_contexts(project_id,version_number,context_document,retained_project_id,retained_version)
        VALUES(local_doc->>'project_id',local_doc->>'version',local_doc,doc->>'project_id',doc->>'version')
        ON CONFLICT (project_id,version_number) DO NOTHING;
        SELECT context_document INTO existing FROM weave_imported_legacy_ontology_contexts
            WHERE project_id=local_doc->>'project_id' AND version_number=local_doc->>'version';
        IF NOT FOUND THEN
            RAISE EXCEPTION 'concurrent imported context requires a fresh snapshot' USING ERRCODE='40001';
        END IF;
        IF existing IS DISTINCT FROM local_doc THEN
            RAISE EXCEPTION 'persisted imported source context conflicts' USING ERRCODE='23514',CONSTRAINT='legacy_ontology_import_conflict';
        END IF;
    END LOOP;
END $$;
-- +goose StatementEnd

-- AFTER INSERT makes the immutable original available to the ledger's FK and
-- derivation guard. A validation failure rolls back the entire INSERT statement,
-- including the import row and every ledger row; no invalid key remains occupied.
-- +goose StatementBegin
CREATE FUNCTION weave_record_imported_legacy_contexts_trigger() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM weave_record_imported_legacy_contexts(NEW.document);
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER record_imported_legacy_contexts AFTER INSERT ON weave_imported_legacy_ontology_snapshots
FOR EACH ROW EXECUTE FUNCTION weave_record_imported_legacy_contexts_trigger();

-- Validate and register existing imports atomically during upgrade. Invalid or
-- conflicting pre-039 imports must be reviewed, never silently grandfathered.
-- +goose StatementBegin
DO $$ DECLARE imported record;
BEGIN
    LOCK TABLE weave_imported_legacy_ontology_snapshots IN SHARE ROW EXCLUSIVE MODE;
    FOR imported IN SELECT document FROM weave_imported_legacy_ontology_snapshots ORDER BY project_id COLLATE "C",version_number COLLATE "C" LOOP
        PERFORM weave_record_imported_legacy_contexts(imported.document);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Keep both imported evidence and the exact body-text capture fix protected even
-- when noncanonical seals came through published restore rather than an import.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM weave_imported_legacy_ontology_snapshots)
       OR EXISTS(SELECT 1 FROM weave_imported_legacy_ontology_contexts)
       OR EXISTS(SELECT 1 FROM weave_ontology_body_seals WHERE body_text IS NOT NULL AND body_text IS DISTINCT FROM body::text) THEN
        RAISE EXCEPTION 'cannot discard imported legacy ontology evidence or exact portable body context';
    END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER record_imported_legacy_contexts ON weave_imported_legacy_ontology_snapshots;
DROP FUNCTION weave_record_imported_legacy_contexts_trigger(),weave_record_imported_legacy_contexts(text);
DROP TABLE weave_imported_legacy_ontology_contexts;
DROP FUNCTION weave_guard_imported_legacy_context_insert(),weave_guard_imported_legacy_context_change(),weave_validate_imported_legacy_context(jsonb),weave_validate_imported_legacy_graph(jsonb),weave_lock_imported_legacy_contexts(text[]),weave_imported_legacy_local_context(jsonb);

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

CREATE OR REPLACE VIEW weave_project_semantic_references AS
WITH refs AS (
    SELECT project_id, version_number, entity_kind, entity_id, location, element,
        CASE lower(COALESCE(NULLIF(element->>'type',''),default_kind))
            WHEN 'literal' THEN 'class' ELSE lower(COALESCE(NULLIF(element->>'type',''),default_kind)) END AS kind
    FROM weave_project_semantic_elements
    WHERE element IS NOT NULL AND element <> 'null'::jsonb AND element <> '{}'::jsonb
      AND lower(COALESCE(element->>'type','')) <> 'complete'
      AND (COALESCE(element->>'type','') <> '' OR COALESCE(element->>'uri','') <> '' OR COALESCE(element->>'prefix','') <> '' OR COALESCE(element->>'local_name','') <> '' OR COALESCE(element->>'ontology_version_id','') <> '')
    UNION ALL SELECT project_id, version_number, entity_kind, entity_id, location || '/additional_types/' || (a.n-1), a.element, 'class'
    FROM weave_project_semantic_elements e
    CROSS JOIN LATERAL jsonb_array_elements(NULLIF(e.element->'additional_types','null'::jsonb)) WITH ORDINALITY a(element,n)
    UNION ALL SELECT project_id, version_number, entity_kind, entity_id, location || '/datatype',
        jsonb_build_object('uri',element->>'datatype'), 'class'
    FROM weave_project_semantic_elements WHERE COALESCE(element->>'datatype','') <> ''
)
SELECT project_id, version_number, entity_kind, entity_id, location,
    jsonb_build_object('kind',kind,'uri',COALESCE(element->>'uri',''),
        'prefix',COALESCE(element->>'prefix',''),'local_name',COALESCE(element->>'local_name',''),
        'version_id',COALESCE(element->>'ontology_version_id','')) AS reference
FROM refs;

DROP FUNCTION weave_legacy_matching_inventory(jsonb),weave_semantic_element_references(jsonb,text,text);
