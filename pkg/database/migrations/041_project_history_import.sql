-- +goose Up
-- Original capture contracts remain 1/2. This immutable ledger records that the
-- rows/evidence arrived through an import rather than a new local hot-table cut.
CREATE TABLE weave_release_imports (
 project_id text NOT NULL,
 version_number text NOT NULL,
 document jsonb NOT NULL,
 imported_at timestamptz NOT NULL DEFAULT now(),
 import_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 PRIMARY KEY(project_id,version_number),
 FOREIGN KEY(project_id,version_number) REFERENCES weave_releases(project_id,version) ON DELETE RESTRICT
);
-- +goose StatementBegin
CREATE FUNCTION weave_project_import_rows_sql(tab text) RETURNS text LANGUAGE plpgsql
SET search_path=public,pg_temp AS $$
DECLARE cols text;
BEGIN
 IF tab NOT IN (SELECT archive_table FROM weave_content_sections_v1())
    AND tab NOT IN (SELECT replace(archive_table,'_archive','') FROM weave_content_sections_v1() WHERE archive_table<>'weave_release_namespace_bindings_archive')
    AND tab<>'weave_namespace_bindings' THEN RAISE EXCEPTION 'unsupported import table' USING ERRCODE='23514'; END IF;
 -- json[b]_populate_record maps JSON null to SQL NULL. JSON columns therefore
 -- arrive as nullable JSON TEXT and are cast separately, retaining JSON null.
 SELECT string_agg(CASE WHEN atttypid IN ('json'::regtype,'jsonb'::regtype)
    THEN format('(doc->>%L)::%s AS %I',attname,atttypid::regtype,attname)
    ELSE format('r.%I',attname) END,',' ORDER BY attnum) INTO cols
 FROM pg_attribute WHERE attrelid=tab::regclass AND attnum>0 AND NOT attisdropped;
 RETURN format('SELECT %s FROM jsonb_array_elements($1) doc CROSS JOIN LATERAL jsonb_populate_record(NULL::%I,doc) r',cols,tab);
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION weave_check_imported_archive_rows(payload jsonb) RETURNS void LANGUAGE plpgsql
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE s record; actual text; expected text; differs boolean;
BEGIN
 FOR s IN SELECT * FROM weave_content_sections_v1() LOOP
  -- Compare typed rows too: to_jsonb(row) receipts cannot distinguish SQL NULL
  -- from a JSONB null. EXCEPT on the actual typed columns retains that distinction.
  actual:=replace(weave_content_archive_sql_v1(s.archive_table,s.owner_column,s.parent_table,s.parent_column),'SELECT to_jsonb(a) AS body','SELECT a.*');
  actual:=replace(actual,'$1','$2');
  -- The actual query needs project/version at $2/$3, while payload rows use $1.
  actual:=replace(actual,'a.version_number=$2','a.version_number=$3');
  expected:=weave_project_import_rows_sql(s.archive_table);
  EXECUTE format('SELECT EXISTS((%s EXCEPT ALL %s) UNION ALL (%s EXCEPT ALL %s))',expected,actual,actual,expected)
   INTO differs USING payload->'sections'->s.archive_table,payload->'header'->>'project_id',payload->'header'->>'version';
  IF differs THEN RAISE EXCEPTION 'typed archive rows differ from imported values: %',s.archive_table USING ERRCODE='23514',CONSTRAINT='project_import_invalid'; END IF;
 END LOOP;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_validate_release_import_sections(payload jsonb) RETURNS void LANGUAGE plpgsql
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE s record; columns text[]; required text[]; row jsonb; total bigint; digest text; expected jsonb;
BEGIN
 IF (payload->>'format_version') IS DISTINCT FROM '1' OR jsonb_typeof(payload->'sections') IS DISTINCT FROM 'object'
    OR (SELECT count(*) FROM jsonb_object_keys(payload->'sections'))<>22
    OR jsonb_typeof(payload->'receipt'->'sections') IS DISTINCT FROM 'object'
    OR (SELECT count(*) FROM jsonb_object_keys(payload->'receipt'->'sections'))<>22 THEN
  RAISE EXCEPTION 'incomplete project import inventory' USING ERRCODE='23514',CONSTRAINT='project_import_invalid';
 END IF;
 FOR s IN SELECT * FROM weave_content_sections_v1() LOOP
  IF jsonb_typeof(payload->'sections'->s.archive_table) IS DISTINCT FROM 'array' THEN
   RAISE EXCEPTION 'missing project import section %',s.archive_table USING ERRCODE='23514',CONSTRAINT='project_import_invalid';
  END IF;
  SELECT array_agg(attname::text ORDER BY attname),array_agg(attname::text ORDER BY attname) FILTER(WHERE attnotnull)
   INTO columns,required FROM pg_attribute WHERE attrelid=s.archive_table::regclass AND attnum>0 AND NOT attisdropped;
  FOR row IN SELECT value FROM jsonb_array_elements(payload->'sections'->s.archive_table) LOOP
   IF jsonb_typeof(row) IS DISTINCT FROM 'object' OR (SELECT array_agg(k ORDER BY k) FROM jsonb_object_keys(row) k) IS DISTINCT FROM columns
      OR EXISTS(SELECT 1 FROM unnest(required) k WHERE row->k='null'::jsonb)
      OR row->>'version_number' IS DISTINCT FROM payload->'header'->>'version' THEN
    RAISE EXCEPTION 'invalid project import row shape %',s.archive_table USING ERRCODE='23514',CONSTRAINT='project_import_invalid';
   END IF;
  END LOOP;
  EXECUTE format('SELECT count(*),encode(sha256(convert_to(COALESCE(string_agg(to_jsonb(r)::text,E''\n'' ORDER BY to_jsonb(r)::text COLLATE "C"),''''),''UTF8'')),''hex'') FROM (%s) r',weave_project_import_rows_sql(s.archive_table))
   INTO total,digest USING payload->'sections'->s.archive_table;
  expected:=jsonb_build_object('rows',total,'sha256',digest);
  IF expected IS DISTINCT FROM payload->'receipt'->'sections'->s.archive_table THEN
   RAISE EXCEPTION 'imported rows differ from original receipt: %',s.archive_table USING ERRCODE='23514',CONSTRAINT='project_import_invalid';
  END IF;
 END LOOP;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_guard_release_imports() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'import provenance is immutable' USING ERRCODE='23514',CONSTRAINT='project_import_immutable'; END IF;
 IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable')
    OR NOT EXISTS(SELECT 1 FROM weave_releases WHERE project_id=NEW.project_id AND version=NEW.version_number
      AND content_capture_version=0 AND archive_ownership_version=0 AND content_capture_xid=pg_current_xact_id())
    OR NEW.document->'header'->>'project_id' IS DISTINCT FROM NEW.project_id
    OR NEW.document->'header'->>'version' IS DISTINCT FROM NEW.version_number
    OR COALESCE(NEW.document->'receipt'->>'format_version','') NOT IN ('1','2') THEN
  RAISE EXCEPTION 'history import requires a new unfinished release and original identity' USING ERRCODE='23514',CONSTRAINT='project_import_invalid';
 END IF;
 NEW.import_xid:=pg_current_xact_id(); NEW.imported_at:=transaction_timestamp();
 RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER guard_release_imports BEFORE INSERT OR UPDATE OR DELETE ON weave_release_imports
 FOR EACH ROW EXECUTE FUNCTION weave_guard_release_imports();
CREATE TRIGGER guard_release_imports_truncate BEFORE TRUNCATE ON weave_release_imports
 FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_release_imports();
-- +goose StatementBegin
CREATE FUNCTION weave_finish_release_import() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM weave_releases WHERE project_id=NEW.project_id AND version=NEW.version_number
  AND content_capture_receipt=NEW.document->'receipt' AND content_capture_version IN (1,2)
  AND archive_ownership_version=1 AND metadata_capture_version=1 AND project_dependencies_version=1) THEN
  RAISE EXCEPTION 'unfinished history import cannot commit' USING ERRCODE='23514',CONSTRAINT='project_import_incomplete';
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER finish_release_import AFTER INSERT ON weave_release_imports
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION weave_finish_release_import();
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_finish_content_capture() RETURNS trigger LANGUAGE plpgsql
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE s record; actual_sql text; differs boolean; total bigint; digest text; sections jsonb:='{}'; snapshot_digest text; imported jsonb;
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
    SELECT document INTO imported FROM weave_release_imports WHERE project_id=NEW.project_id AND version_number=NEW.version;
    IF imported IS NOT NULL AND (imported->'header'->>'created_by_id' IS DISTINCT FROM NEW.created_by_id
       OR (imported->'header'->>'created_at')::timestamptz IS DISTINCT FROM NEW.created_at
       OR (imported->'receipt'->>'format_version')::integer<>NEW.content_capture_version) THEN
        RAISE EXCEPTION 'imported release identity changed' USING ERRCODE='23514',CONSTRAINT='project_import_invalid';
    END IF;
    FOR s IN SELECT * FROM weave_content_sections_v1() ORDER BY archive_table LOOP
        actual_sql:=weave_content_archive_sql_v1(s.archive_table,s.owner_column,s.parent_table,s.parent_column);
        IF imported IS NULL THEN
        EXECUTE format('WITH expected AS MATERIALIZED (SELECT to_jsonb(r) AS body FROM (%s) r), actual AS MATERIALIZED (%s)
            SELECT EXISTS((SELECT body FROM expected EXCEPT ALL SELECT body FROM actual) UNION ALL (SELECT body FROM actual EXCEPT ALL SELECT body FROM expected))',s.expected_sql,actual_sql)
            INTO differs USING NEW.project_id,NEW.version;
        IF differs THEN
            RAISE EXCEPTION 'release content differs from cut snapshot: %',s.archive_table USING ERRCODE='23514',CONSTRAINT='release_content_incomplete';
        END IF;
        END IF;
        EXECUTE format('SELECT count(*),encode(sha256(convert_to(COALESCE(string_agg(body::text,E''\n'' ORDER BY body::text COLLATE "C"),''''),''UTF8'')),''hex'') FROM (%s) r',actual_sql)
            INTO total,digest USING NEW.project_id,NEW.version;
        sections:=sections||jsonb_build_object(s.archive_table,jsonb_build_object('rows',total,'sha256',digest));
    END LOOP;
    NEW.content_capture_receipt:=jsonb_build_object('format_version',NEW.content_capture_version,'sections',sections);
    IF NEW.content_capture_version=2 THEN
        NEW.content_capture_receipt:=NEW.content_capture_receipt||jsonb_build_object('evidence_kind','legacy_ontology_snapshot','ontology_snapshot_digest',snapshot_digest);
    END IF;
    IF imported IS NOT NULL AND NEW.content_capture_receipt IS DISTINCT FROM imported->'receipt' THEN
        RAISE EXCEPTION 'restored archive differs from original receipt' USING ERRCODE='23514',CONSTRAINT='project_import_invalid';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_guard_metadata_capture() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.metadata_capture_version <> 0 THEN
            RAISE EXCEPTION 'metadata must be captured before release finalization' USING ERRCODE='23514';
        END IF;
        PERFORM weave_lock_release_metadata(NEW.project_id,NEW.version);
        RETURN NEW;
    END IF;
    IF OLD.metadata_capture_version = 1 THEN
        IF NEW.metadata_capture_version <> 1 OR NEW.archive_ownership_version <> 1
           OR NEW.project_id <> OLD.project_id OR NEW.version <> OLD.version THEN
            RAISE EXCEPTION 'captured release metadata cannot be downgraded or retargeted' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.metadata_capture_version = 0 THEN RETURN NEW; END IF;
    PERFORM weave_lock_release_metadata(NEW.project_id,NEW.version);
    IF OLD.archive_ownership_version <> 0 OR NEW.archive_ownership_version <> 1
       OR current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'metadata capture requires a new repeatable-read release cut' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM weave_release_imports WHERE project_id=NEW.project_id AND version_number=NEW.version) THEN
        IF NEW.content_capture_version NOT IN (1,2) OR NOT EXISTS(SELECT 1 FROM weave_release_imports
          WHERE project_id=NEW.project_id AND version_number=NEW.version AND document->'receipt'=NEW.content_capture_receipt) THEN
            RAISE EXCEPTION 'import metadata requires validated content receipt' USING ERRCODE='23514',CONSTRAINT='project_import_invalid';
        END IF;
        RETURN NEW;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM weave_projects p JOIN weave_projects_archive a ON a.id=p.id
        WHERE p.id=NEW.project_id AND a.version_number=NEW.version
          AND a.concept_namespace IS NOT DISTINCT FROM p.concept_namespace
          AND a.effective_concept_namespace = weave_effective_concept_namespace_v1(p.concept_namespace)
    ) THEN
        RAISE EXCEPTION 'release omits or changes project concept namespace' USING ERRCODE='23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM
          (SELECT id, model_type FROM weave_models WHERE project_id=NEW.project_id) live
        FULL JOIN
          (SELECT id, model_type FROM weave_models_archive WHERE project_id=NEW.project_id AND version_number=NEW.version) archived
        USING (id)
        WHERE live.id IS NULL OR archived.id IS NULL OR live.model_type IS DISTINCT FROM archived.model_type
    ) THEN
        RAISE EXCEPTION 'release omits or changes model types' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_guard_release_descriptors() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'TRUNCATE' THEN
        IF EXISTS (SELECT 1 FROM weave_release_descriptors) THEN
            RAISE EXCEPTION 'cannot truncate release descriptors' USING ERRCODE='23514';
        END IF;
        RETURN NULL;
    END IF;
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'release descriptors are immutable' USING ERRCODE='23514';
    END IF;
    IF TG_OP = 'DELETE' THEN
        IF EXISTS (SELECT 1 FROM weave_releases WHERE project_id=OLD.project_id AND version=OLD.version_number) THEN
            RAISE EXCEPTION 'release descriptors are retained by their release' USING ERRCODE='23514';
        END IF;
        RETURN OLD;
    END IF;
    IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'release descriptors require repeatable read' USING ERRCODE='23514';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM weave_releases WHERE project_id=NEW.project_id AND version=NEW.version_number AND archive_ownership_version=0) THEN
        RAISE EXCEPTION 'descriptors must be captured before finishing the release cut' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM weave_release_actor_references r WHERE r.project_id=NEW.project_id AND r.version_number=NEW.version_number
        AND r.actor_id IS NOT NULL AND btrim(r.actor_id)<>'' AND NEW.document->'actors'->r.actor_id->>'actor_id' IS DISTINCT FROM r.actor_id)
       OR EXISTS(SELECT 1 FROM weave_release_imports i WHERE i.project_id=NEW.project_id AND i.version_number=NEW.version_number
         AND i.document->'descriptors' IS DISTINCT FROM NEW.document) THEN
        RAISE EXCEPTION 'imported descriptor identity is incomplete or changed' USING ERRCODE='23514';
    END IF;
    IF NOT EXISTS(SELECT 1 FROM weave_release_imports WHERE project_id=NEW.project_id AND version_number=NEW.version_number) AND EXISTS (
        SELECT 1 FROM weave_release_actor_references r LEFT JOIN weave_actors a ON a.id=r.actor_id
        WHERE r.project_id=NEW.project_id AND r.version_number=NEW.version_number
          AND r.actor_id IS NOT NULL AND btrim(r.actor_id)<>''
          AND (a.id IS NULL OR NEW.document->'actors'->r.actor_id IS DISTINCT FROM
            jsonb_strip_nulls(jsonb_build_object('actor_id',a.id,'type',NULLIF(a.type,''),
                'slug',NULLIF(a.slug,''),'display_name',NULLIF(a.display_name,''))))
    ) THEN
        RAISE EXCEPTION 'release descriptors omit or change a required actor' USING ERRCODE='23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM weave_release_project_dependencies p
        LEFT JOIN weave_release_descriptors d ON d.project_id=p.source_project_id AND d.version_number=p.source_version
        WHERE p.project_id=NEW.project_id AND p.version_number=NEW.version_number
          AND (d.document IS NULL OR NEW.document->'project_modules'->>p.source_project_id
            IS DISTINCT FROM d.document->'project_modules'->>p.source_project_id)
    ) THEN
        RAISE EXCEPTION 'release descriptors omit or change a source module' USING ERRCODE='23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM weave_release_ontology_dependencies p
        WHERE p.project_id=NEW.project_id AND p.version_number=NEW.version_number
          AND COALESCE(length(NEW.document->'ontology_modules'->>p.ontology_version_id),0)=0
    ) THEN
        RAISE EXCEPTION 'release descriptors omit an ontology module' USING ERRCODE='23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM weave_release_project_dependencies p
        JOIN weave_release_descriptors d ON d.project_id=p.source_project_id AND d.version_number=p.source_version
        CROSS JOIN LATERAL jsonb_each_text(d.document->'ontology_modules') m
        WHERE p.project_id=NEW.project_id AND p.version_number=NEW.version_number
          AND NEW.document->'ontology_modules'->>m.key IS DISTINCT FROM m.value
    ) THEN
        RAISE EXCEPTION 'release descriptors change a source ontology module' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_capture_legacy_ontology_snapshot() RETURNS trigger LANGUAGE plpgsql
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE selected record; sources jsonb; imported jsonb; retained text;
BEGIN
    IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'legacy ontology capture requires repeatable read' USING ERRCODE='23514';
    END IF;
    -- Use the same per-release write fence as archive writers. A stale
    -- repeatable-read capture fails serialization rather than mixing cuts.
    PERFORM weave_lock_release_metadata(NEW.project_id,NEW.version_number);
    PERFORM 1 FROM weave_releases WHERE project_id=NEW.project_id AND version=NEW.version_number AND content_capture_version=0 FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'legacy ontology capture requires an unfinished owning release' USING ERRCODE='23514'; END IF;
    SELECT document INTO imported FROM weave_release_imports WHERE project_id=NEW.project_id AND version_number=NEW.version_number;
    IF imported IS NOT NULL THEN
        SELECT document INTO retained FROM weave_imported_legacy_ontology_snapshots
          WHERE project_id=NEW.project_id AND version_number=NEW.version_number AND digest=imported->'legacy'->>'digest';
        IF retained IS NULL OR retained IS DISTINCT FROM imported->'legacy'->>'json' THEN
          RAISE EXCEPTION 'import lacks exact retained legacy evidence' USING ERRCODE='23514',CONSTRAINT='project_import_invalid';
        END IF;
        NEW.document:=retained; NEW.digest:=imported->'legacy'->>'digest';
        NEW.context_document:=COALESCE(NULLIF(imported->>'legacy_context',''),(retained::jsonb||jsonb_build_object('sources','[]'::jsonb))::text);
        IF NEW.context_document::jsonb IS DISTINCT FROM weave_legacy_context_v1(NEW.project_id,NEW.version_number) THEN
          RAISE EXCEPTION 'imported legacy context differs from archived inputs' USING ERRCODE='23514',CONSTRAINT='project_import_invalid';
        END IF;
        NEW.context_digest:=encode(sha256(convert_to(NEW.context_document,'UTF8')),'hex');
        RETURN NEW;
    END IF;
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

CREATE TABLE weave_project_workspace_imports (
 project_id text PRIMARY KEY REFERENCES weave_projects(id) ON DELETE RESTRICT,
 digest text NOT NULL CHECK(digest ~ '^[0-9a-f]{64}$'),
 imported_at timestamptz NOT NULL DEFAULT now(),
 detached_staging_refs bigint NOT NULL
);
CREATE TRIGGER guard_workspace_imports BEFORE UPDATE OR DELETE ON weave_project_workspace_imports
 FOR EACH ROW EXECUTE FUNCTION weave_guard_release_imports();
CREATE TRIGGER guard_workspace_imports_truncate BEFORE TRUNCATE ON weave_project_workspace_imports
 FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_release_imports();

-- +goose StatementBegin
CREATE FUNCTION weave_import_hot_row(tab text, body jsonb) RETURNS void LANGUAGE plpgsql
SET search_path=public,pg_temp AS $$
DECLARE cols text;
BEGIN
 -- Callers select the table from the fixed capture registry. Omitted hot-only
 -- columns retain their schema defaults; captured columns never silently vanish.
 IF tab NOT IN (SELECT replace(archive_table,'_archive','') FROM weave_content_sections_v1()
                WHERE archive_table<>'weave_release_namespace_bindings_archive') AND tab<>'weave_namespace_bindings' THEN
  RAISE EXCEPTION 'unsupported working-copy import table' USING ERRCODE='23514';
 END IF;
 IF EXISTS(SELECT 1 FROM jsonb_object_keys(body) k WHERE NOT EXISTS(
    SELECT 1 FROM pg_attribute WHERE attrelid=tab::regclass AND attname=k AND attnum>0 AND NOT attisdropped)) THEN
  RAISE EXCEPTION 'unmapped working-copy import field for %',tab USING ERRCODE='23514';
 END IF;
 SELECT string_agg(format('%I',k),',' ORDER BY k) INTO cols FROM jsonb_object_keys(body) k;
 EXECUTE format('INSERT INTO %I (%s) SELECT %s FROM (%s) rows',tab,cols,cols,weave_project_import_rows_sql(tab)) USING jsonb_build_array(body);
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION weave_project_import_working_row(body jsonb, tab text) RETURNS jsonb LANGUAGE plpgsql AS $$
BEGIN
 body:=body-'version_number'-'effective_concept_namespace'-'release_project_id';
 -- The archive denormalizes override ownership; the hot table obtains it
 -- through its override FK and has no project_id column.
 IF tab='weave_override_refs' THEN body:=body-'project_id'; END IF;
 IF body ? 'status' AND tab IN ('weave_projects','weave_fields','weave_models','weave_collections','weave_categories','weave_vocabularies','weave_concept_lists') THEN
  body:=body||jsonb_build_object('status','draft');
 END IF;
 -- Staging is an instance-local attachment, not part of the portable graph.
 -- Retain its exact ID in archives; clear only missing hot-table FK targets.
 IF body ? 'staging_id' AND body->>'staging_id' IS NOT NULL
    AND NOT EXISTS(SELECT 1 FROM weave_import_staging WHERE id=(body->>'staging_id')::bigint) THEN
  body:=body||jsonb_build_object('staging_id',NULL);
 END IF;
 RETURN body;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION weave_prepare_project_import(payload jsonb) RETURNS boolean LANGUAGE plpgsql
SET search_path=public,pg_temp AS $$
DECLARE project text:=payload->'header'->>'project_id'; actor jsonb; row jsonb; incoming_digest text:=encode(sha256(convert_to(payload::text,'UTF8')),'hex');
BEGIN
 IF project IS NULL OR btrim(project)='' OR jsonb_array_length(payload->'sections'->'weave_projects_archive')<>1 THEN
  RAISE EXCEPTION 'project import lacks a project row' USING ERRCODE='23514',CONSTRAINT='project_import_invalid';
 END IF;
 IF EXISTS(SELECT 1 FROM weave_projects WHERE id=project) THEN
  IF NOT EXISTS(SELECT 1 FROM weave_project_workspace_imports i WHERE i.project_id=project AND i.digest=incoming_digest) THEN
   RAISE EXCEPTION 'working copy already exists or differs from this import' USING ERRCODE='23505',CONSTRAINT='project_import_conflict';
  END IF;
  RETURN false;
 END IF;
 FOR actor IN SELECT value FROM jsonb_each(payload->'descriptors'->'actors') LOOP
  INSERT INTO weave_actors(id,type,slug,display_name)
   VALUES(actor->>'actor_id',COALESCE(NULLIF(actor->>'type',''),'person'),COALESCE(NULLIF(actor->>'slug',''),actor->>'actor_id'),COALESCE(actor->>'display_name',''))
   ON CONFLICT(id) DO NOTHING;
 END LOOP;
 row:=payload->'sections'->'weave_projects_archive'->0;
 IF row->>'id' IS DISTINCT FROM project THEN RAISE EXCEPTION 'project identity mismatch' USING ERRCODE='23514'; END IF;
 PERFORM weave_import_hot_row('weave_projects',weave_project_import_working_row(row,'weave_projects'));
 RETURN true;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION weave_import_project_release(payload jsonb) RETURNS boolean LANGUAGE plpgsql
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE project text:=payload->'header'->>'project_id'; release_version text:=payload->'header'->>'version';
 h jsonb:=payload->'header'; existing jsonb; s record; names text[]; tab text; receipt jsonb:=payload->'receipt';
BEGIN
 IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
  RAISE EXCEPTION 'history import requires repeatable read' USING ERRCODE='23514';
 END IF;
 PERFORM weave_lock_release_metadata(project,release_version);
 PERFORM weave_validate_release_import_sections(payload);
 FOREACH tab IN ARRAY ARRAY['dependencies','membership','publications'] LOOP
  IF jsonb_typeof(payload->tab) IS DISTINCT FROM 'array' OR EXISTS(SELECT 1 FROM jsonb_array_elements(payload->tab) r
    WHERE r->>'project_id' IS DISTINCT FROM project OR r->>'version_number' IS DISTINCT FROM release_version) THEN
   RAISE EXCEPTION 'import evidence escapes its project/release' USING ERRCODE='23514',CONSTRAINT='project_import_invalid';
  END IF;
 END LOOP;

 SELECT document INTO existing FROM weave_release_imports WHERE project_id=project AND version_number=release_version;
 IF existing IS NOT NULL THEN
  IF existing IS DISTINCT FROM payload-'sections' THEN
   RAISE EXCEPTION 'release import conflicts with retained history' USING ERRCODE='23505',CONSTRAINT='project_import_conflict';
  END IF;
  PERFORM weave_check_imported_archive_rows(payload);
  RETURN false;
 END IF;
 IF EXISTS(SELECT 1 FROM weave_releases WHERE project_id=project AND version=release_version) THEN
  RAISE EXCEPTION 'release already exists without matching imported provenance' USING ERRCODE='23505',CONSTRAINT='project_import_conflict';
 END IF;
 INSERT INTO weave_releases(project_id,version,title,description,created_at,created_by_id,archived_at,archived_message)
  VALUES(project,release_version,h->>'title',h->>'description',(h->>'created_at')::timestamptz,h->>'created_by_id',(h->>'archived_at')::timestamptz,h->>'archived_message');
 INSERT INTO weave_release_imports(project_id,version_number,document) VALUES(project,release_version,payload-'sections');
 -- Parents precede children, including ownership-only joins used by guards.
 FOR s IN SELECT * FROM weave_content_sections_v1() ORDER BY (parent_table<>''),archive_table LOOP
  EXECUTE format('INSERT INTO %I %s',s.archive_table,weave_project_import_rows_sql(s.archive_table))
   USING payload->'sections'->s.archive_table;
 END LOOP;
 PERFORM weave_check_imported_archive_rows(payload);
 INSERT INTO weave_release_project_dependencies SELECT * FROM jsonb_populate_recordset(NULL::weave_release_project_dependencies,payload->'dependencies');
 INSERT INTO weave_release_dependency_entities SELECT * FROM jsonb_populate_recordset(NULL::weave_release_dependency_entities,payload->'membership');
 IF EXISTS(SELECT 1 FROM weave_release_project_dependencies d JOIN weave_release_project_dependencies transitive
   ON transitive.project_id=d.source_project_id AND transitive.version_number=d.source_version
   WHERE d.project_id=project AND d.version_number=release_version AND NOT EXISTS(
    SELECT 1 FROM weave_release_project_dependencies actual WHERE actual.project_id=project AND actual.version_number=release_version
      AND actual.source_project_id=transitive.source_project_id AND actual.source_version=transitive.source_version)) THEN
  RAISE EXCEPTION 'import source release closure is incomplete' USING ERRCODE='23514',CONSTRAINT='project_import_invalid';
 END IF;

 IF receipt->>'format_version'='1' THEN
  INSERT INTO weave_release_ontology_dependencies SELECT * FROM jsonb_populate_recordset(NULL::weave_release_ontology_dependencies,payload->'publications');
  UPDATE weave_releases SET ontology_dependencies_version=1 WHERE project_id=project AND version=release_version;
  INSERT INTO weave_release_ontology_references(project_id,version_number,resolutions) VALUES(project,release_version,payload->'references');
 ELSE
  INSERT INTO weave_legacy_ontology_snapshots(project_id,version_number,context_document,context_digest,document,digest)
   VALUES(project,release_version,'',repeat('0',64),'',repeat('0',64));
 END IF;
 INSERT INTO weave_release_descriptors(project_id,version_number,document) VALUES(project,release_version,payload->'descriptors');
 UPDATE weave_releases SET archive_ownership_version=1,metadata_capture_version=1,project_dependencies_version=1,
  ontology_dependencies_version=CASE WHEN receipt->>'format_version'='1' THEN 1 ELSE 0 END,
  content_capture_version=(receipt->>'format_version')::smallint WHERE project_id=project AND version=release_version;
 RETURN true;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION weave_restore_project_workspace(payload jsonb) RETURNS bigint LANGUAGE plpgsql
SET search_path=public,pg_temp AS $$
DECLARE project text:=payload->'header'->>'project_id'; section text; tab text; row jsonb; body jsonb; seq text; high bigint; detached bigint:=0;
BEGIN
 IF EXISTS(SELECT 1 FROM weave_project_workspace_imports WHERE project_id=project) THEN
  RAISE EXCEPTION 'working copy import already finished' USING ERRCODE='23505',CONSTRAINT='project_import_conflict';
 END IF;
 -- Explicit FK order; no namespace globals or imported ontology defaults become
 -- instance policy. Releases retain their entire captured namespace inventories.
 FOREACH section IN ARRAY ARRAY['weave_projects_archive','weave_categories_archive','weave_fields_archive','weave_models_archive',
  'weave_collections_archive','weave_vocabularies_archive','weave_vocabulary_entries_archive','weave_concept_lists_archive',
  'weave_concept_list_entries_archive','weave_concept_broader_archive','weave_field_overrides_archive','weave_override_refs_archive',
  'weave_collection_placements_archive','weave_examples_archive','weave_example_values_archive','weave_project_actors_archive',
  'weave_project_attributions_archive','weave_project_inheritance_archive','weave_project_ontology_versions_archive',
  'weave_adoptions_archive','weave_entity_forks_archive','weave_release_namespace_bindings_archive'] LOOP
  tab:=replace(section,'_archive',''); IF section='weave_release_namespace_bindings_archive' THEN tab:='weave_namespace_bindings'; END IF;
  seq:=NULL;
  IF EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=tab::regclass AND attname='id' AND attnum>0 AND NOT attisdropped) THEN
   seq:=pg_get_serial_sequence(tab,'id');
  END IF;
  IF seq IS NOT NULL THEN
   -- Ordinary INSERT takes ROW EXCLUSIVE before allocating its default ID.
   -- Block those inserts until the explicit IDs and sequence frontier agree.
   EXECUTE format('LOCK TABLE %I IN SHARE ROW EXCLUSIVE MODE',tab);
  END IF;
  FOR row IN SELECT value FROM jsonb_array_elements(payload->'sections'->section) LOOP
   body:=weave_project_import_working_row(row,tab);
   IF row->>'staging_id' IS NOT NULL AND body->>'staging_id' IS NULL THEN detached:=detached+1; END IF;
   IF section='weave_projects_archive' THEN CONTINUE; END IF;
   IF section='weave_release_namespace_bindings_archive' AND row->>'project_id' IS DISTINCT FROM project THEN CONTINUE; END IF;
   PERFORM weave_import_hot_row(tab,body);
  END LOOP;
  -- Sequences are nontransactional. Advance monotonically; rollback may leave
  -- harmless gaps, never reuse a restored identity or rewind an existing counter.
  seq:=NULL;
  IF EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=tab::regclass AND attname='id' AND attnum>0 AND NOT attisdropped) THEN
   seq:=pg_get_serial_sequence(tab,'id');
  END IF;
  IF seq IS NOT NULL THEN
   EXECUTE format('SELECT max(id) FROM %I',tab) INTO high;
   IF high IS NOT NULL THEN PERFORM setval(seq::regclass,GREATEST(high,nextval(seq::regclass)),true); END IF;
  END IF;
 END LOOP;
 INSERT INTO weave_project_workspace_imports(project_id,digest,detached_staging_refs)
  VALUES(project,encode(sha256(convert_to(payload::text,'UTF8')),'hex'),detached);
 RETURN detached;
END $$;
-- +goose StatementEnd


-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM weave_release_imports) OR EXISTS(SELECT 1 FROM weave_project_workspace_imports) THEN RAISE EXCEPTION 'cannot downgrade retained imported release history'; END IF;
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
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_guard_metadata_capture() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.metadata_capture_version <> 0 THEN
            RAISE EXCEPTION 'metadata must be captured before release finalization' USING ERRCODE='23514';
        END IF;
        PERFORM weave_lock_release_metadata(NEW.project_id,NEW.version);
        RETURN NEW;
    END IF;
    IF OLD.metadata_capture_version = 1 THEN
        IF NEW.metadata_capture_version <> 1 OR NEW.archive_ownership_version <> 1
           OR NEW.project_id <> OLD.project_id OR NEW.version <> OLD.version THEN
            RAISE EXCEPTION 'captured release metadata cannot be downgraded or retargeted' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.metadata_capture_version = 0 THEN RETURN NEW; END IF;
    PERFORM weave_lock_release_metadata(NEW.project_id,NEW.version);
    IF OLD.archive_ownership_version <> 0 OR NEW.archive_ownership_version <> 1
       OR current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'metadata capture requires a new repeatable-read release cut' USING ERRCODE='23514';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM weave_projects p JOIN weave_projects_archive a ON a.id=p.id
        WHERE p.id=NEW.project_id AND a.version_number=NEW.version
          AND a.concept_namespace IS NOT DISTINCT FROM p.concept_namespace
          AND a.effective_concept_namespace = weave_effective_concept_namespace_v1(p.concept_namespace)
    ) THEN
        RAISE EXCEPTION 'release omits or changes project concept namespace' USING ERRCODE='23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM
          (SELECT id, model_type FROM weave_models WHERE project_id=NEW.project_id) live
        FULL JOIN
          (SELECT id, model_type FROM weave_models_archive WHERE project_id=NEW.project_id AND version_number=NEW.version) archived
        USING (id)
        WHERE live.id IS NULL OR archived.id IS NULL OR live.model_type IS DISTINCT FROM archived.model_type
    ) THEN
        RAISE EXCEPTION 'release omits or changes model types' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_guard_release_descriptors() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'TRUNCATE' THEN
        IF EXISTS (SELECT 1 FROM weave_release_descriptors) THEN
            RAISE EXCEPTION 'cannot truncate release descriptors' USING ERRCODE='23514';
        END IF;
        RETURN NULL;
    END IF;
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'release descriptors are immutable' USING ERRCODE='23514';
    END IF;
    IF TG_OP = 'DELETE' THEN
        IF EXISTS (SELECT 1 FROM weave_releases WHERE project_id=OLD.project_id AND version=OLD.version_number) THEN
            RAISE EXCEPTION 'release descriptors are retained by their release' USING ERRCODE='23514';
        END IF;
        RETURN OLD;
    END IF;
    IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
        RAISE EXCEPTION 'release descriptors require repeatable read' USING ERRCODE='23514';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM weave_releases WHERE project_id=NEW.project_id AND version=NEW.version_number AND archive_ownership_version=0) THEN
        RAISE EXCEPTION 'descriptors must be captured before finishing the release cut' USING ERRCODE='23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM weave_release_actor_references r LEFT JOIN weave_actors a ON a.id=r.actor_id
        WHERE r.project_id=NEW.project_id AND r.version_number=NEW.version_number
          AND r.actor_id IS NOT NULL AND btrim(r.actor_id)<>''
          AND (a.id IS NULL OR NEW.document->'actors'->r.actor_id IS DISTINCT FROM
            jsonb_strip_nulls(jsonb_build_object('actor_id',a.id,'type',NULLIF(a.type,''),
                'slug',NULLIF(a.slug,''),'display_name',NULLIF(a.display_name,''))))
    ) THEN
        RAISE EXCEPTION 'release descriptors omit or change a required actor' USING ERRCODE='23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM weave_release_project_dependencies p
        LEFT JOIN weave_release_descriptors d ON d.project_id=p.source_project_id AND d.version_number=p.source_version
        WHERE p.project_id=NEW.project_id AND p.version_number=NEW.version_number
          AND (d.document IS NULL OR NEW.document->'project_modules'->>p.source_project_id
            IS DISTINCT FROM d.document->'project_modules'->>p.source_project_id)
    ) THEN
        RAISE EXCEPTION 'release descriptors omit or change a source module' USING ERRCODE='23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM weave_release_ontology_dependencies p
        WHERE p.project_id=NEW.project_id AND p.version_number=NEW.version_number
          AND COALESCE(length(NEW.document->'ontology_modules'->>p.ontology_version_id),0)=0
    ) THEN
        RAISE EXCEPTION 'release descriptors omit an ontology module' USING ERRCODE='23514';
    END IF;
    IF EXISTS (
        SELECT 1 FROM weave_release_project_dependencies p
        JOIN weave_release_descriptors d ON d.project_id=p.source_project_id AND d.version_number=p.source_version
        CROSS JOIN LATERAL jsonb_each_text(d.document->'ontology_modules') m
        WHERE p.project_id=NEW.project_id AND p.version_number=NEW.version_number
          AND NEW.document->'ontology_modules'->>m.key IS DISTINCT FROM m.value
    ) THEN
        RAISE EXCEPTION 'release descriptors change a source ontology module' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
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
DROP TABLE weave_project_workspace_imports;
DROP FUNCTION weave_restore_project_workspace(jsonb),weave_import_project_release(jsonb),weave_prepare_project_import(jsonb),weave_project_import_working_row(jsonb,text),weave_import_hot_row(text,jsonb);
DROP TABLE weave_release_imports;
DROP FUNCTION weave_guard_release_imports(),weave_finish_release_import(),weave_validate_release_import_sections(jsonb);

DROP FUNCTION weave_check_imported_archive_rows(jsonb),weave_project_import_rows_sql(text);
