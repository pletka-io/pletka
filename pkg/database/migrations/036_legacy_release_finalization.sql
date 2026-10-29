-- +goose Up
-- Version 1 remains verified publication. Version 2 preserves legacy semantic
-- evidence while certifying the same complete immutable structural sections.
ALTER TABLE weave_releases DROP CONSTRAINT weave_releases_content_capture_version_check;
ALTER TABLE weave_releases ADD CONSTRAINT weave_releases_content_capture_version_check CHECK (content_capture_version IN (0,1,2));
-- +goose StatementBegin
CREATE FUNCTION weave_legacy_context_v1(project text, release_version text) RETURNS jsonb LANGUAGE plpgsql STABLE
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
        'digest',s.digest,'body_json',s.body::text,'selection',to_jsonb(l)) ORDER BY s.version_id COLLATE "C"),'[]')
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
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_capture_legacy_ontology_snapshot() RETURNS trigger LANGUAGE plpgsql
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE selected record; sources jsonb;
BEGIN
    IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'legacy ontology capture requires repeatable read' USING ERRCODE='23514';
    END IF;
    -- Use the same per-release write fence as archive writers. A stale
    -- repeatable-read capture fails serialization rather than mixing cuts.
    PERFORM weave_lock_release_metadata(NEW.project_id,NEW.version_number);
    PERFORM 1 FROM weave_releases WHERE project_id=NEW.project_id AND version=NEW.version_number AND content_capture_version=0 FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'legacy ontology capture requires an unfinished owning release' USING ERRCODE='23514'; END IF;
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
    NEW.context_document := weave_legacy_context_v1(NEW.project_id,NEW.version_number)::text;
    NEW.context_digest := encode(sha256(convert_to(NEW.context_document,'UTF8')),'hex');
    NEW.document := (NEW.context_document::jsonb || jsonb_build_object('sources',sources))::text;
    NEW.digest := encode(sha256(convert_to(NEW.document,'UTF8')),'hex');
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_guard_content_rows() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE body jsonb; owner text; parent_owner text; release_version text;
BEGIN
    IF TG_OP='TRUNCATE' THEN
        RAISE EXCEPTION 'cannot truncate release content' USING ERRCODE='23514';
    END IF;
    FOR body IN SELECT value FROM jsonb_array_elements(
        CASE TG_OP WHEN 'INSERT' THEN jsonb_build_array(to_jsonb(NEW))
        WHEN 'DELETE' THEN jsonb_build_array(to_jsonb(OLD))
        ELSE jsonb_build_array(to_jsonb(OLD),to_jsonb(NEW)) END)
    LOOP
        release_version:=body->>'version_number';
        IF TG_ARGV[3]='true' THEN
            PERFORM weave_lock_content_parent(TG_TABLE_NAME,body->>'id',release_version);
        END IF;
        parent_owner:=NULL;
        IF TG_ARGV[1]<>'' THEN
            PERFORM weave_lock_content_parent(TG_ARGV[1],body->>TG_ARGV[2],release_version);
            EXECUTE format('SELECT project_id FROM %I WHERE id::text=$1 AND version_number=$2',TG_ARGV[1])
                INTO parent_owner USING body->>TG_ARGV[2],release_version;
        END IF;
        FOR owner IN SELECT DISTINCT x FROM unnest(ARRAY[body->>TG_ARGV[0],parent_owner]) x WHERE x IS NOT NULL LOOP
            PERFORM weave_lock_release_metadata(owner,release_version);
            IF EXISTS(SELECT 1 FROM weave_releases WHERE project_id=owner AND weave_releases.version=release_version
                AND (content_capture_version IN (1,2)
                    OR (TG_TABLE_NAME IN ('weave_release_project_dependencies','weave_release_dependency_entities') AND project_dependencies_version=1)
                    OR (TG_TABLE_NAME='weave_release_ontology_dependencies' AND ontology_dependencies_version=1))) THEN
                RAISE EXCEPTION 'captured release content is immutable' USING ERRCODE='23514',CONSTRAINT='release_content_immutable';
            END IF;
        END LOOP;
    END LOOP;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_finish_content_capture() RETURNS trigger LANGUAGE plpgsql
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE s record; actual_sql text; differs boolean; total bigint; digest text; sections jsonb:='{}'; snapshot_digest text;
BEGIN
    IF TG_OP='DELETE' THEN
        PERFORM weave_lock_release_metadata(OLD.project_id,OLD.version);
        RETURN OLD;
    END IF;
    IF TG_OP='INSERT' THEN
        IF NEW.content_capture_version<>0 OR NEW.content_capture_receipt IS NOT NULL THEN
            RAISE EXCEPTION 'content must be captured before release finalization' USING ERRCODE='23514';
        END IF;
        NEW.content_capture_xid:=pg_current_xact_id();
        RETURN NEW;
    END IF;
    IF NEW.content_capture_xid IS DISTINCT FROM OLD.content_capture_xid
       OR NEW.content_capture_receipt IS DISTINCT FROM OLD.content_capture_receipt THEN
        RAISE EXCEPTION 'content receipt and capture transaction are storage-owned' USING ERRCODE='23514';
    END IF;
    IF OLD.project_dependencies_version=1 AND (NEW.project_dependencies_version<>1
       OR NEW.project_id<>OLD.project_id OR NEW.version<>OLD.version) THEN
        RAISE EXCEPTION 'captured project dependency evidence cannot be reset or retargeted' USING ERRCODE='23514';
    END IF;
    IF OLD.content_capture_version IN (1,2) THEN
        IF NEW.content_capture_version<>OLD.content_capture_version OR NEW.project_id<>OLD.project_id OR NEW.version<>OLD.version
           OR NEW.archive_ownership_version<>1 OR NEW.metadata_capture_version<>1
           OR NEW.project_dependencies_version<>1 OR NEW.ontology_dependencies_version<>OLD.ontology_dependencies_version
           OR NEW.created_by_id IS DISTINCT FROM OLD.created_by_id OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
            RAISE EXCEPTION 'captured release content cannot be downgraded or retargeted' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.content_capture_version=0 THEN RETURN NEW; END IF;
    IF OLD.content_capture_xid IS DISTINCT FROM pg_current_xact_id()
       OR NEW.project_id<>OLD.project_id OR NEW.version<>OLD.version
       OR OLD.archive_ownership_version<>0 OR NEW.archive_ownership_version<>1
       OR NEW.metadata_capture_version<>1 OR NEW.project_dependencies_version<>1
       OR current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'content capture requires a new repeatable-read cut and complete prerequisite evidence' USING ERRCODE='23514';
    END IF;
    PERFORM weave_lock_release_metadata(NEW.project_id,NEW.version);
    IF NOT EXISTS(SELECT 1 FROM weave_release_descriptors WHERE project_id=NEW.project_id AND version_number=NEW.version) THEN
        RAISE EXCEPTION 'content capture lacks descriptors' USING ERRCODE='23514';
    END IF;
    IF NEW.content_capture_version=1 THEN
        IF NEW.ontology_dependencies_version<>1
           OR NOT EXISTS(SELECT 1 FROM weave_release_ontology_references WHERE project_id=NEW.project_id AND version_number=NEW.version)
           OR EXISTS(SELECT 1 FROM weave_legacy_ontology_snapshots WHERE project_id=NEW.project_id AND version_number=NEW.version) THEN
            RAISE EXCEPTION 'verified content capture requires verified ontology evidence' USING ERRCODE='23514';
        END IF;
    ELSIF NEW.content_capture_version=2 THEN
        IF NEW.ontology_dependencies_version<>0
           OR EXISTS(SELECT 1 FROM weave_release_ontology_references WHERE project_id=NEW.project_id AND version_number=NEW.version)
           OR EXISTS(SELECT 1 FROM weave_release_ontology_dependencies WHERE project_id=NEW.project_id AND version_number=NEW.version) THEN
            RAISE EXCEPTION 'legacy and verified ontology evidence must remain distinct' USING ERRCODE='23514';
        END IF;
        SELECT l.digest INTO snapshot_digest FROM weave_legacy_ontology_snapshots l
            WHERE l.project_id=NEW.project_id AND l.version_number=NEW.version
              AND l.context_document::jsonb=weave_legacy_context_v1(NEW.project_id,NEW.version);
        IF snapshot_digest IS NULL THEN
            RAISE EXCEPTION 'legacy ontology evidence is missing or differs from the cut' USING ERRCODE='23514',CONSTRAINT='legacy_context_incomplete';
        END IF;
        IF EXISTS(SELECT 1 FROM weave_legacy_ontology_snapshots l, LATERAL jsonb_array_elements(l.context_document::jsonb->'inventory') b
            WHERE l.project_id=NEW.project_id AND l.version_number=NEW.version AND NOT EXISTS (
                SELECT 1 FROM weave_release_descriptors d WHERE d.project_id=NEW.project_id AND d.version_number=NEW.version
                AND COALESCE(length(d.document->'ontology_modules'->>(b->>'version_id')),0)>0)) THEN
            RAISE EXCEPTION 'legacy content capture omits an ontology module' USING ERRCODE='23514';
        END IF;
    ELSE
        RAISE EXCEPTION 'unsupported content capture contract' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM weave_release_project_dependencies d JOIN weave_releases r ON r.project_id=d.source_project_id AND r.version=d.source_version
        WHERE d.project_id=NEW.project_id AND d.version_number=NEW.version AND r.content_capture_version<>NEW.content_capture_version) THEN
        RAISE EXCEPTION 'source release content has a different or missing capture contract' USING ERRCODE='23514',CONSTRAINT='release_content_source_unverified';
    END IF;
    FOR s IN SELECT * FROM weave_content_sections_v1() ORDER BY archive_table LOOP
        actual_sql:=weave_content_archive_sql_v1(s.archive_table,s.owner_column,s.parent_table,s.parent_column);
        EXECUTE format('WITH expected AS MATERIALIZED (SELECT to_jsonb(r) AS body FROM (%s) r), actual AS MATERIALIZED (%s)
            SELECT EXISTS((SELECT body FROM expected EXCEPT ALL SELECT body FROM actual) UNION ALL (SELECT body FROM actual EXCEPT ALL SELECT body FROM expected))',s.expected_sql,actual_sql)
            INTO differs USING NEW.project_id,NEW.version;
        IF differs THEN
            RAISE EXCEPTION 'release content differs from cut snapshot: %',s.archive_table USING ERRCODE='23514',CONSTRAINT='release_content_incomplete';
        END IF;
        EXECUTE format('SELECT count(*),encode(sha256(convert_to(COALESCE(string_agg(body::text,E''\n'' ORDER BY body::text COLLATE "C"),''''),''UTF8'')),''hex'') FROM (%s) r',actual_sql)
            INTO total,digest USING NEW.project_id,NEW.version;
        sections:=sections||jsonb_build_object(s.archive_table,jsonb_build_object('rows',total,'sha256',digest));
    END LOOP;
    NEW.content_capture_receipt:=jsonb_build_object('format_version',NEW.content_capture_version,'sections',sections);
    IF NEW.content_capture_version=2 THEN
        NEW.content_capture_receipt:=NEW.content_capture_receipt||jsonb_build_object('evidence_kind','legacy_ontology_snapshot','ontology_snapshot_digest',snapshot_digest);
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- Plan fingerprints also bind mutable unpublished ontology input and immutable
-- evidence. Checking only published artifacts misses legacy body edits.
-- +goose StatementBegin
CREATE FUNCTION weave_release_rebuild_fingerprint_v1(projects text[]) RETURNS text LANGUAGE plpgsql
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE p record; s record; rows_digest text; evidence text:='';
BEGIN
    FOR p IN SELECT * FROM weave_releases WHERE project_id=ANY(projects) ORDER BY project_id,version LOOP
        evidence:=evidence||to_jsonb(p)::text;
        FOR s IN SELECT * FROM weave_content_sections_v1() ORDER BY archive_table LOOP
            EXECUTE format('SELECT encode(sha256(convert_to(COALESCE(string_agg(to_jsonb(r)::text,E''\n'' ORDER BY to_jsonb(r)::text COLLATE "C"),''''),''UTF8'')),''hex'') FROM (%s) r',s.expected_sql)
                INTO rows_digest USING p.project_id,p.version;
            evidence:=evidence||s.archive_table||rows_digest;
        END LOOP;
    END LOOP;
    -- Include policy/identity inputs that select artifacts and descriptors, and
    -- the full catalog/retention graph. Store only a digest, never credentials.
    SELECT COALESCE(string_agg(body::text,E'\n' ORDER BY body::text COLLATE "C"),'') INTO rows_digest FROM (
        SELECT jsonb_build_array('actor',id,type,slug,display_name) body FROM weave_actors
        UNION ALL SELECT jsonb_build_array('ontology',to_jsonb(o)) FROM weave_ontologies o
        UNION ALL SELECT jsonb_build_array('active',to_jsonb(a)) FROM weave_ontology_active_versions a
        UNION ALL SELECT jsonb_build_array('artifact',ontology_id,version_id,digest) FROM weave_ontology_releases
        UNION ALL SELECT jsonb_build_array('release',to_jsonb(r)) FROM weave_releases r
        UNION ALL SELECT jsonb_build_array('pin',to_jsonb(d)) FROM weave_release_project_dependencies d
    ) inputs;
    RETURN encode(sha256(convert_to(evidence||rows_digest,'UTF8')),'hex');
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_release_rebuild_fingerprint(projects text[]) RETURNS text LANGUAGE sql
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
    WITH inputs AS (
        SELECT jsonb_build_array('version',to_jsonb(v)) body FROM weave_ontology_versions v
        UNION ALL SELECT jsonb_build_array('class',to_jsonb(c)) FROM weave_ontology_classes c
        UNION ALL SELECT jsonb_build_array('property',to_jsonb(p)) FROM weave_ontology_properties p
        UNION ALL SELECT jsonb_build_array('relation',to_jsonb(r)) FROM weave_ontology_relations r
        UNION ALL SELECT jsonb_build_array('companion',to_jsonb(c)) FROM weave_ontology_version_companions c
        UNION ALL SELECT jsonb_build_array('body_seal',version_id,digest) FROM weave_ontology_body_seals
        UNION ALL SELECT jsonb_build_array('legacy_snapshot',project_id,version_number,digest) FROM weave_legacy_ontology_snapshots
    ) SELECT encode(sha256(convert_to(weave_release_rebuild_fingerprint_v1(projects)||
        COALESCE(string_agg(body::text,E'\n' ORDER BY body::text COLLATE "C"),''),'UTF8')),'hex') FROM inputs;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM weave_releases WHERE content_capture_version=2) OR EXISTS(SELECT 1 FROM weave_release_rebuild_operations WHERE (document::jsonb->'plan'->>'format')='2') THEN
        RAISE EXCEPTION 'cannot downgrade retained legacy content captures or operations';
    END IF;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_finish_content_capture() RETURNS trigger LANGUAGE plpgsql
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE s record; actual_sql text; differs boolean; total bigint; digest text; sections jsonb:='{}';
BEGIN
    IF TG_OP='DELETE' THEN
        PERFORM weave_lock_release_metadata(OLD.project_id,OLD.version);
        RETURN OLD;
    END IF;
    IF TG_OP='INSERT' THEN
        IF NEW.content_capture_version<>0 OR NEW.content_capture_receipt IS NOT NULL THEN
            RAISE EXCEPTION 'content must be captured before release finalization' USING ERRCODE='23514';
        END IF;
        NEW.content_capture_xid:=pg_current_xact_id();
        RETURN NEW;
    END IF;
    IF NEW.content_capture_xid IS DISTINCT FROM OLD.content_capture_xid
       OR NEW.content_capture_receipt IS DISTINCT FROM OLD.content_capture_receipt THEN
        RAISE EXCEPTION 'content receipt and capture transaction are storage-owned' USING ERRCODE='23514';
    END IF;
    IF OLD.project_dependencies_version=1 AND (NEW.project_dependencies_version<>1
       OR NEW.project_id<>OLD.project_id OR NEW.version<>OLD.version) THEN
        RAISE EXCEPTION 'captured project dependency evidence cannot be reset or retargeted' USING ERRCODE='23514';
    END IF;
    IF OLD.content_capture_version=1 THEN
        IF NEW.content_capture_version<>1 OR NEW.project_id<>OLD.project_id OR NEW.version<>OLD.version
           OR NEW.archive_ownership_version<>1 OR NEW.metadata_capture_version<>1
           OR NEW.project_dependencies_version<>1 OR NEW.ontology_dependencies_version<>1
           OR NEW.created_by_id IS DISTINCT FROM OLD.created_by_id OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
            RAISE EXCEPTION 'captured release content cannot be downgraded or retargeted' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.content_capture_version=0 THEN RETURN NEW; END IF;
    IF OLD.content_capture_xid IS DISTINCT FROM pg_current_xact_id()
       OR NEW.project_id<>OLD.project_id OR NEW.version<>OLD.version
       OR OLD.archive_ownership_version<>0 OR NEW.archive_ownership_version<>1
       OR NEW.metadata_capture_version<>1 OR NEW.project_dependencies_version<>1 OR NEW.ontology_dependencies_version<>1
       OR current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'content capture requires a new repeatable-read cut and complete prerequisite evidence' USING ERRCODE='23514';
    END IF;
    PERFORM weave_lock_release_metadata(NEW.project_id,NEW.version);
    IF NOT EXISTS(SELECT 1 FROM weave_release_descriptors WHERE project_id=NEW.project_id AND version_number=NEW.version)
       OR NOT EXISTS(SELECT 1 FROM weave_release_ontology_references WHERE project_id=NEW.project_id AND version_number=NEW.version) THEN
        RAISE EXCEPTION 'content capture lacks descriptors or semantic evidence' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM weave_release_project_dependencies d JOIN weave_releases r ON r.project_id=d.source_project_id AND r.version=d.source_version
        WHERE d.project_id=NEW.project_id AND d.version_number=NEW.version AND r.content_capture_version<>1) THEN
        RAISE EXCEPTION 'source release content is unverified' USING ERRCODE='23514',CONSTRAINT='release_content_source_unverified';
    END IF;
    FOR s IN SELECT * FROM weave_content_sections_v1() ORDER BY archive_table LOOP
        actual_sql:=weave_content_archive_sql_v1(s.archive_table,s.owner_column,s.parent_table,s.parent_column);
        EXECUTE format('WITH expected AS MATERIALIZED (SELECT to_jsonb(r) AS body FROM (%s) r), actual AS MATERIALIZED (%s)
            SELECT EXISTS((SELECT body FROM expected EXCEPT ALL SELECT body FROM actual) UNION ALL (SELECT body FROM actual EXCEPT ALL SELECT body FROM expected))',s.expected_sql,actual_sql)
            INTO differs USING NEW.project_id,NEW.version;
        IF differs THEN
            RAISE EXCEPTION 'release content differs from cut snapshot: %',s.archive_table USING ERRCODE='23514',CONSTRAINT='release_content_incomplete';
        END IF;
        EXECUTE format('SELECT count(*),encode(sha256(convert_to(COALESCE(string_agg(body::text,E''\n'' ORDER BY body::text COLLATE "C"),''''),''UTF8'')),''hex'') FROM (%s) r',actual_sql)
            INTO total,digest USING NEW.project_id,NEW.version;
        sections:=sections||jsonb_build_object(s.archive_table,jsonb_build_object('rows',total,'sha256',digest));
    END LOOP;
    NEW.content_capture_receipt:=jsonb_build_object('format_version',1,'sections',sections);
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_guard_content_rows() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE body jsonb; owner text; parent_owner text; release_version text;
BEGIN
    IF TG_OP='TRUNCATE' THEN
        RAISE EXCEPTION 'cannot truncate release content' USING ERRCODE='23514';
    END IF;
    FOR body IN SELECT value FROM jsonb_array_elements(
        CASE TG_OP WHEN 'INSERT' THEN jsonb_build_array(to_jsonb(NEW))
        WHEN 'DELETE' THEN jsonb_build_array(to_jsonb(OLD))
        ELSE jsonb_build_array(to_jsonb(OLD),to_jsonb(NEW)) END)
    LOOP
        release_version:=body->>'version_number';
        IF TG_ARGV[3]='true' THEN
            PERFORM weave_lock_content_parent(TG_TABLE_NAME,body->>'id',release_version);
        END IF;
        parent_owner:=NULL;
        IF TG_ARGV[1]<>'' THEN
            PERFORM weave_lock_content_parent(TG_ARGV[1],body->>TG_ARGV[2],release_version);
            EXECUTE format('SELECT project_id FROM %I WHERE id::text=$1 AND version_number=$2',TG_ARGV[1])
                INTO parent_owner USING body->>TG_ARGV[2],release_version;
        END IF;
        FOR owner IN SELECT DISTINCT x FROM unnest(ARRAY[body->>TG_ARGV[0],parent_owner]) x WHERE x IS NOT NULL LOOP
            PERFORM weave_lock_release_metadata(owner,release_version);
            IF EXISTS(SELECT 1 FROM weave_releases WHERE project_id=owner AND weave_releases.version=release_version
                AND (content_capture_version=1
                    OR (TG_TABLE_NAME IN ('weave_release_project_dependencies','weave_release_dependency_entities') AND project_dependencies_version=1)
                    OR (TG_TABLE_NAME='weave_release_ontology_dependencies' AND ontology_dependencies_version=1))) THEN
                RAISE EXCEPTION 'captured release content is immutable' USING ERRCODE='23514',CONSTRAINT='release_content_immutable';
            END IF;
        END LOOP;
    END LOOP;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_capture_legacy_ontology_snapshot() RETURNS trigger LANGUAGE plpgsql
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
DROP FUNCTION weave_legacy_context_v1(text,text);
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_release_rebuild_fingerprint(projects text[]) RETURNS text LANGUAGE plpgsql
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE p record; s record; rows_digest text; evidence text:='';
BEGIN
    FOR p IN SELECT * FROM weave_releases WHERE project_id=ANY(projects) ORDER BY project_id,version LOOP
        evidence:=evidence||to_jsonb(p)::text;
        FOR s IN SELECT * FROM weave_content_sections_v1() ORDER BY archive_table LOOP
            EXECUTE format('SELECT encode(sha256(convert_to(COALESCE(string_agg(to_jsonb(r)::text,E''\n'' ORDER BY to_jsonb(r)::text COLLATE "C"),''''),''UTF8'')),''hex'') FROM (%s) r',s.expected_sql)
                INTO rows_digest USING p.project_id,p.version;
            evidence:=evidence||s.archive_table||rows_digest;
        END LOOP;
    END LOOP;
    -- Include policy/identity inputs that select artifacts and descriptors, and
    -- the full catalog/retention graph. Store only a digest, never credentials.
    SELECT COALESCE(string_agg(body::text,E'\n' ORDER BY body::text COLLATE "C"),'') INTO rows_digest FROM (
        SELECT jsonb_build_array('actor',id,type,slug,display_name) body FROM weave_actors
        UNION ALL SELECT jsonb_build_array('ontology',to_jsonb(o)) FROM weave_ontologies o
        UNION ALL SELECT jsonb_build_array('active',to_jsonb(a)) FROM weave_ontology_active_versions a
        UNION ALL SELECT jsonb_build_array('artifact',ontology_id,version_id,digest) FROM weave_ontology_releases
        UNION ALL SELECT jsonb_build_array('release',to_jsonb(r)) FROM weave_releases r
        UNION ALL SELECT jsonb_build_array('pin',to_jsonb(d)) FROM weave_release_project_dependencies d
    ) inputs;
    RETURN encode(sha256(convert_to(evidence||rows_digest,'UTF8')),'hex');
END $$;
-- +goose StatementEnd
DROP FUNCTION weave_release_rebuild_fingerprint_v1(text[]);
ALTER TABLE weave_releases DROP CONSTRAINT weave_releases_content_capture_version_check;
ALTER TABLE weave_releases ADD CONSTRAINT weave_releases_content_capture_version_check CHECK (content_capture_version IN (0,1));
