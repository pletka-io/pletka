-- +goose Up
-- Separate evidence contract: this never sets a verified publication or project
-- content marker. Existing archives are inputs, not certified by this receipt.
CREATE TABLE weave_legacy_ontology_snapshots (
    project_id text NOT NULL,
    version_number text NOT NULL,
    context_document text NOT NULL,
    context_digest text NOT NULL CHECK (context_digest ~ '^[0-9a-f]{64}$'),
    document text NOT NULL,
    digest text NOT NULL CHECK (digest ~ '^[0-9a-f]{64}$'),
    PRIMARY KEY (project_id,version_number),
    FOREIGN KEY (project_id,version_number) REFERENCES weave_releases(project_id,version) ON DELETE RESTRICT
);

-- Produce assessments from values only. The same function checks the complete
-- retained candidate set; primary flags and catalog/active policy are irrelevant.
-- +goose StatementBegin
CREATE FUNCTION weave_legacy_ontology_assessment(ref jsonb, inventory jsonb, aliases jsonb) RETURNS jsonb
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
CREATE FUNCTION weave_capture_legacy_ontology_snapshot() RETURNS trigger LANGUAGE plpgsql
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE selected record; inventory jsonb; bindings jsonb; elements jsonb; assessments jsonb; sources jsonb; dependencies jsonb; source_refs jsonb;
BEGIN
    IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'legacy ontology capture requires repeatable read' USING ERRCODE='23514';
    END IF;
    -- Use the same per-release write fence as archive writers. A stale
    -- repeatable-read capture fails serialization rather than mixing cuts.
    PERFORM weave_lock_release_metadata(NEW.project_id,NEW.version_number);
    PERFORM 1 FROM weave_releases WHERE project_id=NEW.project_id AND version=NEW.version_number FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'legacy ontology capture requires an owning release' USING ERRCODE='23514'; END IF;
    IF EXISTS (SELECT 1 FROM weave_release_project_dependencies d
        LEFT JOIN weave_legacy_ontology_snapshots s ON s.project_id=d.source_project_id AND s.version_number=d.source_version
        WHERE d.project_id=NEW.project_id AND d.version_number=NEW.version_number AND s.project_id IS NULL) THEN
        RAISE EXCEPTION 'source project has no retained legacy ontology snapshot' USING ERRCODE='23514';
    END IF;
    FOR selected IN
        SELECT l.ontology_version_id,v.ontology_id FROM weave_project_ontology_versions_archive l
        LEFT JOIN weave_ontology_versions v ON v.id=l.ontology_version_id
        WHERE l.project_id=NEW.project_id AND l.version_number=NEW.version_number ORDER BY l.ontology_version_id COLLATE "C"
    LOOP
        IF selected.ontology_id IS NULL THEN RAISE EXCEPTION 'selected ontology body is missing' USING ERRCODE='23514'; END IF;
        IF NOT EXISTS(SELECT 1 FROM weave_ontology_body_seals WHERE version_id=selected.ontology_version_id) THEN
            INSERT INTO weave_ontology_body_seals(ontology_id,version_id) VALUES (selected.ontology_id,selected.ontology_version_id);
        END IF;
    END LOOP;
    SELECT COALESCE(jsonb_agg(jsonb_build_object('ontology_id',s.ontology_id,'version_id',s.version_id,
        'digest',s.digest,'body_json',s.body::text,'selection',to_jsonb(l)) ORDER BY s.version_id COLLATE "C"),'[]')
        INTO inventory FROM weave_project_ontology_versions_archive l
        JOIN weave_ontology_body_seals s ON s.version_id=l.ontology_version_id
        WHERE l.project_id=NEW.project_id AND l.version_number=NEW.version_number;
    SELECT COALESCE(jsonb_agg(to_jsonb(b) ORDER BY b.id COLLATE "C"),'[]') INTO bindings
        FROM weave_release_namespace_bindings_archive b WHERE b.release_project_id=NEW.project_id AND b.version_number=NEW.version_number;
    -- Retain original type/datatype/URI/QName/version values, including elements
    -- that have no reference. Normalization below never rewrites these inputs.
    SELECT COALESCE(jsonb_agg(jsonb_build_object('entity_kind',entity_kind,'entity_id',entity_id,
        'location',location,'default_kind',default_kind,'original',element)
        ORDER BY entity_kind COLLATE "C",entity_id COLLATE "C",location COLLATE "C"),'[]') INTO elements
        FROM weave_project_semantic_elements WHERE project_id=NEW.project_id AND version_number=NEW.version_number;
    SELECT COALESCE(jsonb_agg(jsonb_build_object('entity_kind',entity_kind,'entity_id',entity_id,'location',location,
        'reference',reference) || weave_legacy_ontology_assessment(reference,inventory,bindings)
        ORDER BY entity_kind COLLATE "C",entity_id COLLATE "C",location COLLATE "C"),'[]') INTO assessments
        FROM weave_project_semantic_references WHERE project_id=NEW.project_id AND version_number=NEW.version_number;
    SELECT COALESCE(jsonb_agg(jsonb_build_object('project_id',s.project_id,'version',s.version_number,'digest',s.context_digest)
        ORDER BY s.project_id COLLATE "C",s.version_number COLLATE "C"),'[]') INTO source_refs
        FROM weave_release_project_dependencies d JOIN weave_legacy_ontology_snapshots s
          ON s.project_id=d.source_project_id AND s.version_number=d.source_version
        WHERE d.project_id=NEW.project_id AND d.version_number=NEW.version_number;
    -- Flatten the retained context closure. Recursively embedding each complete
    -- source snapshot would grow exponentially for transitive project graphs.
    WITH selected_sources AS (
        SELECT s.* FROM weave_release_project_dependencies d JOIN weave_legacy_ontology_snapshots s
          ON s.project_id=d.source_project_id AND s.version_number=d.source_version
        WHERE d.project_id=NEW.project_id AND d.version_number=NEW.version_number
    ), contexts AS (
        SELECT jsonb_build_object('json',context_document,'digest',context_digest) AS item FROM selected_sources
        UNION SELECT item FROM selected_sources, LATERAL jsonb_array_elements(document::jsonb->'sources') item
    ) SELECT COALESCE(jsonb_agg(item ORDER BY item::text COLLATE "C"),'[]') INTO sources FROM contexts;
    SELECT COALESCE(jsonb_agg(to_jsonb(e) ORDER BY entity_type COLLATE "C",entity_id COLLATE "C"),'[]') INTO dependencies
        FROM weave_release_dependency_entities e WHERE project_id=NEW.project_id AND version_number=NEW.version_number;
    NEW.context_document := jsonb_build_object('format_version',1,'project_id',NEW.project_id,'version',NEW.version_number,
        'inventory',inventory,'bindings',bindings,'elements',elements,'assessments',assessments,'sources','[]'::jsonb,'source_refs',source_refs,'dependencies',dependencies)::text;
    NEW.context_digest := encode(sha256(convert_to(NEW.context_document,'UTF8')),'hex');
    NEW.document := (NEW.context_document::jsonb || jsonb_build_object('sources',sources))::text;
    NEW.digest := encode(sha256(convert_to(NEW.document,'UTF8')),'hex');
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER capture_legacy_ontology_snapshot BEFORE INSERT ON weave_legacy_ontology_snapshots
FOR EACH ROW EXECUTE FUNCTION weave_capture_legacy_ontology_snapshot();

-- +goose StatementBegin
CREATE FUNCTION weave_guard_legacy_ontology_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP<>'TRUNCATE' OR EXISTS(SELECT 1 FROM weave_legacy_ontology_snapshots) THEN
        RAISE EXCEPTION 'legacy ontology snapshots are retained and immutable' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_legacy_ontology_snapshot BEFORE UPDATE OR DELETE ON weave_legacy_ontology_snapshots
FOR EACH ROW EXECUTE FUNCTION weave_guard_legacy_ontology_snapshot();
CREATE TRIGGER guard_legacy_ontology_snapshot_truncate BEFORE TRUNCATE ON weave_legacy_ontology_snapshots
FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_legacy_ontology_snapshot();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM weave_legacy_ontology_snapshots) THEN
        RAISE EXCEPTION 'cannot discard retained legacy ontology snapshots';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE weave_legacy_ontology_snapshots;
DROP FUNCTION weave_guard_legacy_ontology_snapshot(),weave_capture_legacy_ontology_snapshot(),weave_legacy_ontology_assessment(jsonb,jsonb,jsonb);
