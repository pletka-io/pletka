-- +goose Up
-- Opt-in enrollment: existing projects retain their legacy workflow. Managed
-- projects cannot mutate outside one open receipt in the writing transaction.
CREATE TABLE weave_draft_blobs (
 digest text PRIMARY KEY CHECK(digest=encode(sha256(body),'hex')),
 body bytea NOT NULL
);
CREATE TABLE weave_draft_snapshots (
 digest text PRIMARY KEY CHECK(digest=encode(sha256(convert_to(manifest::text,'UTF8')),'hex')),
 manifest jsonb NOT NULL CHECK(jsonb_typeof(manifest)='object')
);
CREATE TABLE weave_draft_projects (
 project_id text PRIMARY KEY,
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 snapshot_digest text NOT NULL REFERENCES weave_draft_snapshots(digest)
);
CREATE TABLE weave_draft_mutations (
 project_id text NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),
 transaction_id xid8 NOT NULL DEFAULT pg_current_xact_id(),
 purpose text NOT NULL CHECK(purpose IN ('edit','context_refresh')),
 actor_id text NOT NULL, actor_name text NOT NULL, actor_email text NOT NULL,
 message text NOT NULL CHECK(btrim(message)<>''),
 before_digest text NOT NULL REFERENCES weave_draft_snapshots(digest),
 after_digest text REFERENCES weave_draft_snapshots(digest),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(project_id,revision), UNIQUE(transaction_id)
);
CREATE TABLE weave_draft_effects (
 id bigserial PRIMARY KEY,
 project_id text NOT NULL, revision bigint NOT NULL, table_name text NOT NULL,
 before_row jsonb, after_row jsonb,
 FOREIGN KEY(project_id,revision) REFERENCES weave_draft_mutations(project_id,revision),
 CHECK(before_row IS NOT NULL OR after_row IS NOT NULL),
 CHECK(before_row IS NULL OR jsonb_typeof(before_row)='object'),
 CHECK(after_row IS NULL OR jsonb_typeof(after_row)='object')
);
-- Parent tombstones keep cascade ownership available after the parent DELETE.
CREATE TABLE weave_draft_row_owners (
 table_name text NOT NULL, entity_id text NOT NULL, project_id text NOT NULL,
 PRIMARY KEY(table_name,entity_id)
);
CREATE TABLE weave_draft_project_fences (key text PRIMARY KEY);
-- +goose StatementBegin
CREATE FUNCTION weave_draft_fence(identity text) RETURNS void LANGUAGE sql AS $$
 INSERT INTO weave_draft_project_fences VALUES(identity) ON CONFLICT(key) DO UPDATE SET key=EXCLUDED.key;
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_lock_draft_capture(project text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_advisory_lock_shared(hashtext('project:'||project));
 PERFORM pg_advisory_lock_shared(731,421);
 PERFORM pg_advisory_lock(hashtextextended('pletka-draft-project:'||project,731));
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_lock_entity_in_transaction(project_key text, entity_key text, budget_ms bigint) RETURNS void LANGUAGE plpgsql AS $$
DECLARE previous_timeout text:=current_setting('lock_timeout'); started timestamptz:=clock_timestamp(); remaining bigint;
BEGIN
 PERFORM set_config('lock_timeout',greatest(1,budget_ms)::text||'ms',true);
 PERFORM pg_advisory_xact_lock_shared(hashtext(project_key));
 remaining:=greatest(1,budget_ms-(extract(epoch FROM clock_timestamp()-started)*1000)::bigint);
 PERFORM set_config('lock_timeout',remaining::text||'ms',true);
 PERFORM pg_advisory_xact_lock(hashtext(entity_key));
 PERFORM set_config('lock_timeout',previous_timeout,true);
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_draft_sections_v1() RETURNS TABLE(
 hot_table text, archive_table text, owner_column text, parent_table text, parent_column text
) LANGUAGE sql IMMUTABLE AS $$ VALUES
('weave_adoptions','weave_adoptions_archive','project_id','',''),
('weave_categories','weave_categories_archive','project_id','',''),
('weave_collection_placements','weave_collection_placements_archive','project_id','',''),
('weave_collections','weave_collections_archive','project_id','',''),
('weave_concept_broader','weave_concept_broader_archive','','weave_concept_lists','scheme_id'),
('weave_concept_list_entries','weave_concept_list_entries_archive','','weave_concept_lists','concept_list_id'),
('weave_concept_lists','weave_concept_lists_archive','project_id','',''),
('weave_entity_forks','weave_entity_forks_archive','project_id','',''),
('weave_example_values','weave_example_values_archive','','weave_examples','example_id'),
('weave_examples','weave_examples_archive','project_id','',''),
('weave_field_overrides','weave_field_overrides_archive','project_id','',''),
('weave_fields','weave_fields_archive','project_id','',''),
('weave_models','weave_models_archive','project_id','',''),
('weave_namespace_bindings','weave_release_namespace_bindings_archive','project_id','',''),
('weave_override_refs','weave_override_refs_archive','','weave_field_overrides','override_id'),
('weave_project_actors','weave_project_actors_archive','project_id','',''),
('weave_project_attributions','weave_project_attributions_archive','project_id','',''),
('weave_project_inheritance','weave_project_inheritance_archive','project_id','',''),
('weave_project_ontology_versions','weave_project_ontology_versions_archive','project_id','',''),
('weave_projects','weave_projects_archive','id','',''),
('weave_vocabularies','weave_vocabularies_archive','project_id','',''),
('weave_vocabulary_entries','weave_vocabulary_entries_archive','','weave_vocabularies','vocabulary_id'); $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION weave_draft_lock_key(project text) RETURNS bigint LANGUAGE sql IMMUTABLE AS $$
 SELECT hashtextextended('pletka-draft-project:'||project,731);
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_draft_row_image(body anyelement, tab regclass) RETURNS jsonb
LANGUAGE plpgsql SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO,YMD' AS $$
DECLARE cols text; result jsonb;
BEGIN
 -- Preserve SQL NULL vs authored JSON null as nullable JSON text. Numeric,
 -- dates and timestamps are text; no float conversion or locale-dependent time.
 SELECT string_agg(format('%L, %s',attname,CASE
  WHEN atttypid IN ('json'::regtype,'jsonb'::regtype,'numeric'::regtype,'date'::regtype)
   THEN format('($1).%I::text',attname)
  WHEN atttypid='timestamptz'::regtype THEN format(
   '(CASE WHEN isfinite(($1).%1$I) THEN to_char(($1).%1$I AT TIME ZONE ''UTC'',''YYYY-MM-DD"T"HH24:MI:SS.US"Z"'') || CASE WHEN EXTRACT(YEAR FROM ($1).%1$I AT TIME ZONE ''UTC'')<0 THEN '' BC'' ELSE '''' END ELSE ($1).%1$I::text END)',attname)
  ELSE format('($1).%I',attname) END),',' ORDER BY attnum) INTO cols
 FROM pg_attribute WHERE attrelid=tab AND attnum>0 AND NOT attisdropped;
 EXECUTE 'SELECT jsonb_build_object('||cols||')' INTO result USING body;
 RETURN result;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_draft_owner(tab text, body jsonb) RETURNS text LANGUAGE plpgsql
SET search_path=public,pg_temp AS $$
DECLARE spec record; owner text;
BEGIN
 SELECT * INTO STRICT spec FROM weave_draft_sections_v1() WHERE hot_table=tab;
 IF spec.owner_column<>'' THEN RETURN body->>spec.owner_column; END IF;
 IF spec.parent_column='scheme_id' AND body->>spec.parent_column IS NULL THEN RETURN ''; END IF;
 EXECUTE format('SELECT COALESCE(project_id,'''') FROM %I WHERE id::text=$1',spec.parent_table)
 INTO owner USING body->>spec.parent_column;
 IF owner IS NULL THEN
  SELECT project_id INTO owner FROM weave_draft_row_owners
  WHERE table_name=spec.parent_table AND entity_id=body->>spec.parent_column;
 END IF;
 IF owner IS NULL THEN RAISE EXCEPTION 'missing draft row owner for %',tab USING ERRCODE='23514'; END IF;
 RETURN owner;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_guard_draft_write() RETURNS trigger LANGUAGE plpgsql SET search_path=public,pg_temp AS $$
DECLARE body jsonb; owner text; other_owner text; lock_owner text; managed boolean; active weave_draft_mutations;
BEGIN
 IF TG_OP='TRUNCATE' THEN
  PERFORM weave_draft_fence('enrollment');
  IF EXISTS(SELECT 1 FROM weave_draft_projects) THEN RAISE EXCEPTION 'cannot truncate captured draft tables' USING ERRCODE='23514'; END IF;
  RETURN NULL;
 END IF;
 body:=CASE WHEN TG_OP='DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
 owner:=weave_draft_owner(TG_TABLE_NAME,body);
 other_owner:=owner;
 IF TG_OP='UPDATE' THEN other_owner:=weave_draft_owner(TG_TABLE_NAME,to_jsonb(OLD)); END IF;
 SELECT * INTO active FROM weave_draft_mutations WHERE transaction_id=pg_current_xact_id() AND after_digest IS NULL LIMIT 1;
 -- Reject a lost callback context before it waits on its own owner's lock.
 -- The post-lock check and MVCC fences remain authoritative for old snapshots.
 IF EXISTS(SELECT 1 FROM weave_draft_projects WHERE project_id IN(owner,other_owner))
    AND (active.project_id IS NULL OR active.project_id IS DISTINCT FROM owner OR active.project_id IS DISTINCT FROM other_owner) THEN
  RAISE EXCEPTION 'project % requires atomic draft capture',owner USING ERRCODE='23514',CONSTRAINT='draft_capture_required';
 END IF;
 IF COALESCE(owner,'')='' OR COALESCE(other_owner,'')='' THEN
  IF active.project_id IS NOT NULL THEN RAISE EXCEPTION 'global inputs require a separate context operation' USING ERRCODE='23514'; END IF;
  PERFORM pg_advisory_xact_lock(731,421);
 ELSE
  PERFORM pg_advisory_xact_lock_shared(731,421);
 END IF;
 -- Fence both sides of a move before consulting enrollment. An old RR snapshot
 -- must not move a row OUT of a project enrolled after that snapshot started.
 FOR lock_owner IN SELECT DISTINCT p FROM unnest(ARRAY[owner,other_owner]) p WHERE COALESCE(p,'')<>'' ORDER BY p LOOP
  PERFORM pg_advisory_xact_lock(weave_draft_lock_key(lock_owner));
  PERFORM weave_draft_fence('p:'||lock_owner);
 END LOOP;
 IF owner IS DISTINCT FROM other_owner AND EXISTS(SELECT 1 FROM weave_draft_projects WHERE project_id IN(owner,other_owner)) THEN
  RAISE EXCEPTION 'captured rows cannot move between projects' USING ERRCODE='23514';
 END IF;
 SELECT EXISTS(SELECT 1 FROM weave_draft_projects WHERE project_id=owner) INTO managed;
 IF active.project_id IS NOT NULL AND active.project_id<>owner THEN RAISE EXCEPTION 'draft transaction cannot write another project' USING ERRCODE='23514'; END IF;
 IF managed AND (active.project_id IS NULL OR active.project_id<>owner) THEN
  RAISE EXCEPTION 'project % requires atomic draft capture',owner USING ERRCODE='23514',CONSTRAINT='draft_capture_required';
 END IF;
 IF managed AND TG_TABLE_NAME='weave_projects' AND TG_OP='DELETE' THEN
  RAISE EXCEPTION 'captured project deletion requires an explicit terminal revision' USING ERRCODE='23514';
 END IF;
 IF TG_TABLE_NAME IN ('weave_field_overrides','weave_examples','weave_concept_lists','weave_vocabularies') THEN
  INSERT INTO weave_draft_row_owners VALUES(TG_TABLE_NAME,body->>'id',COALESCE(owner,''))
  ON CONFLICT(table_name,entity_id) DO UPDATE SET project_id=EXCLUDED.project_id;
 END IF;
 RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_capture_draft_effect() RETURNS trigger LANGUAGE plpgsql SET search_path=public,pg_temp AS $$
DECLARE active weave_draft_mutations; prior jsonb; next jsonb; owner text;
BEGIN
 IF NOT EXISTS(SELECT 1 FROM weave_draft_mutations WHERE transaction_id=pg_current_xact_id() AND after_digest IS NULL) THEN RETURN NULL; END IF;
 IF TG_OP<>'INSERT' THEN prior:=weave_draft_row_image(OLD,TG_RELID); END IF;
 IF TG_OP<>'DELETE' THEN next:=weave_draft_row_image(NEW,TG_RELID); END IF;
 owner:=weave_draft_owner(TG_TABLE_NAME,COALESCE(next,prior));
 SELECT * INTO active FROM weave_draft_mutations WHERE project_id=owner AND transaction_id=pg_current_xact_id() AND after_digest IS NULL;
 IF FOUND THEN INSERT INTO weave_draft_effects(project_id,revision,table_name,before_row,after_row)
  VALUES(owner,active.revision,TG_TABLE_NAME,prior,next); END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_guard_draft_global_input() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM weave_draft_mutations WHERE transaction_id=pg_current_xact_id() AND after_digest IS NULL) THEN
  RAISE EXCEPTION 'global input writes cannot be attributed to one project edit' USING ERRCODE='23514';
 END IF;
 -- Capture holds this shared lock before opening its repeatable-read snapshot.
 PERFORM pg_advisory_xact_lock(731,421);
 RETURN NULL;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
DO $$ DECLARE s record; tab text; BEGIN
 FOR s IN SELECT * FROM weave_draft_sections_v1() LOOP
  EXECUTE format('CREATE TRIGGER guard_draft_capture BEFORE INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION weave_guard_draft_write()',s.hot_table);
  EXECUTE format('CREATE TRIGGER capture_draft_effect AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION weave_capture_draft_effect()',s.hot_table);
  EXECUTE format('CREATE TRIGGER guard_draft_truncate BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_draft_write()',s.hot_table);
 END LOOP;
 FOREACH tab IN ARRAY ARRAY['weave_field_overrides','weave_examples','weave_concept_lists','weave_vocabularies'] LOOP
  EXECUTE format('INSERT INTO weave_draft_row_owners SELECT %L,id::text,COALESCE(project_id,'''') FROM %I',tab,tab);
 END LOOP;
 FOREACH tab IN ARRAY ARRAY['weave_actors','weave_ontologies','weave_ontology_versions','weave_ontology_active_versions','weave_ontology_releases'] LOOP
  EXECUTE format('CREATE TRIGGER guard_draft_global_input BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_draft_global_input()',tab);
 END LOOP;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION weave_store_draft_snapshot(files jsonb) RETURNS text LANGUAGE plpgsql SET search_path=public,pg_temp AS $$
DECLARE manifest jsonb; snapshot_id text;
BEGIN
 IF jsonb_typeof(files)<>'object' OR NOT files ? 'project.yaml' THEN RAISE EXCEPTION 'missing complete draft snapshot' USING ERRCODE='23514'; END IF;
 INSERT INTO weave_draft_blobs(digest,body)
 SELECT encode(sha256(decode(value#>>'{}','base64')),'hex'),decode(value#>>'{}','base64') FROM jsonb_each(files)
 ON CONFLICT(digest) DO NOTHING;
 SELECT jsonb_object_agg(key,encode(sha256(decode(value#>>'{}','base64')),'hex')) INTO manifest FROM jsonb_each(files);
 snapshot_id:=encode(sha256(convert_to(manifest::text,'UTF8')),'hex');
 INSERT INTO weave_draft_snapshots VALUES(snapshot_id,manifest) ON CONFLICT(digest) DO NOTHING;
 RETURN snapshot_id;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_guard_draft_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'draft capture history is immutable' USING ERRCODE='23514'; END $$;
-- +goose StatementEnd
CREATE TRIGGER immutable_draft_blobs BEFORE UPDATE OR DELETE OR TRUNCATE ON weave_draft_blobs FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_draft_immutable();
CREATE TRIGGER immutable_draft_snapshots BEFORE UPDATE OR DELETE OR TRUNCATE ON weave_draft_snapshots FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_draft_immutable();
CREATE TRIGGER immutable_draft_effects BEFORE UPDATE OR DELETE OR TRUNCATE ON weave_draft_effects FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_draft_immutable();
-- +goose StatementBegin
CREATE FUNCTION weave_guard_draft_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  IF NEW.transaction_id<>pg_current_xact_id() OR NEW.after_digest IS NOT NULL OR NOT EXISTS(
   SELECT 1 FROM weave_draft_projects p WHERE p.project_id=NEW.project_id AND p.revision+1=NEW.revision AND p.snapshot_digest=NEW.before_digest
  ) THEN RAISE EXCEPTION 'invalid draft revision start' USING ERRCODE='23514'; END IF;
 ELSIF TG_OP='UPDATE' THEN
  IF OLD.after_digest IS NOT NULL OR OLD.transaction_id<>pg_current_xact_id() OR NEW.after_digest IS NULL OR
   (to_jsonb(OLD)-'after_digest') IS DISTINCT FROM (to_jsonb(NEW)-'after_digest') THEN
   RAISE EXCEPTION 'draft receipt cannot be rewritten' USING ERRCODE='23514';
  END IF;
 ELSE RAISE EXCEPTION 'draft receipt cannot be removed' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_draft_receipt BEFORE INSERT OR UPDATE OR DELETE ON weave_draft_mutations FOR EACH ROW EXECUTE FUNCTION weave_guard_draft_receipt();
CREATE TRIGGER guard_draft_receipt_truncate BEFORE TRUNCATE ON weave_draft_mutations FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_draft_immutable();
-- +goose StatementBegin
CREATE FUNCTION weave_check_draft_receipt_closed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM weave_draft_mutations m JOIN weave_draft_projects p USING(project_id)
  WHERE m.project_id=NEW.project_id AND m.revision=NEW.revision AND m.after_digest IS NOT NULL
  AND p.revision=m.revision AND p.snapshot_digest=m.after_digest) THEN
  RAISE EXCEPTION 'draft mutation must finalize before commit' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER draft_receipt_closed AFTER INSERT ON weave_draft_mutations
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION weave_check_draft_receipt_closed();

-- +goose StatementBegin
CREATE FUNCTION weave_guard_draft_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  PERFORM weave_draft_fence('enrollment');
  PERFORM weave_draft_fence('p:'||NEW.project_id);
  IF NEW.revision<>0 OR NOT EXISTS(SELECT 1 FROM weave_projects WHERE id=NEW.project_id) THEN
   RAISE EXCEPTION 'invalid initial draft state' USING ERRCODE='23514';
  END IF;
 ELSIF TG_OP='UPDATE' THEN
  IF NEW.project_id<>OLD.project_id OR NEW.revision<>OLD.revision+1 OR NOT EXISTS(
   SELECT 1 FROM weave_draft_mutations m WHERE m.project_id=NEW.project_id AND m.revision=NEW.revision
    AND m.before_digest=OLD.snapshot_digest AND m.after_digest=NEW.snapshot_digest AND m.transaction_id=pg_current_xact_id()
  ) THEN RAISE EXCEPTION 'draft state must advance with its receipt' USING ERRCODE='23514'; END IF;
 ELSE RAISE EXCEPTION 'enrolled draft state cannot be removed' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_draft_state BEFORE INSERT OR UPDATE OR DELETE ON weave_draft_projects FOR EACH ROW EXECUTE FUNCTION weave_guard_draft_state();
CREATE TRIGGER guard_draft_state_truncate BEFORE TRUNCATE ON weave_draft_projects FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_draft_immutable();
-- +goose StatementBegin
CREATE FUNCTION weave_guard_draft_owners() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF pg_trigger_depth()<2 OR TG_OP='DELETE' THEN RAISE EXCEPTION 'draft parent ownership is maintained by row capture' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_draft_owners BEFORE INSERT OR UPDATE OR DELETE ON weave_draft_row_owners FOR EACH ROW EXECUTE FUNCTION weave_guard_draft_owners();
CREATE TRIGGER guard_draft_owners_truncate BEFORE TRUNCATE ON weave_draft_row_owners FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_draft_immutable();
-- +goose StatementBegin
CREATE FUNCTION weave_guard_draft_effect_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF pg_trigger_depth()<2 OR NOT EXISTS(SELECT 1 FROM weave_draft_sections_v1() WHERE hot_table=NEW.table_name) OR NOT EXISTS(
  SELECT 1 FROM weave_draft_mutations WHERE project_id=NEW.project_id AND revision=NEW.revision AND transaction_id=pg_current_xact_id() AND after_digest IS NULL
 ) THEN RAISE EXCEPTION 'draft effects require an open capture transaction' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_draft_effect_insert BEFORE INSERT ON weave_draft_effects FOR EACH ROW EXECUTE FUNCTION weave_guard_draft_effect_insert();

-- +goose StatementBegin
CREATE FUNCTION weave_guard_draft_fences() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND NEW.key=OLD.key THEN RETURN NEW; END IF;
 RAISE EXCEPTION 'draft coordination fences cannot be removed or retargeted' USING ERRCODE='23514';
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_draft_fences BEFORE UPDATE OR DELETE ON weave_draft_project_fences FOR EACH ROW EXECUTE FUNCTION weave_guard_draft_fences();
CREATE TRIGGER guard_draft_fences_truncate BEFORE TRUNCATE ON weave_draft_project_fences FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_draft_fences();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM weave_draft_projects) THEN
  RAISE EXCEPTION 'cannot downgrade retained atomic draft history' USING ERRCODE='23514';
 END IF;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
DO $$ DECLARE s record; tab text; BEGIN
 FOR s IN SELECT * FROM weave_draft_sections_v1() LOOP
  EXECUTE format('DROP TRIGGER guard_draft_capture ON %I',s.hot_table);
  EXECUTE format('DROP TRIGGER capture_draft_effect ON %I',s.hot_table);
  EXECUTE format('DROP TRIGGER guard_draft_truncate ON %I',s.hot_table);
 END LOOP;
 FOREACH tab IN ARRAY ARRAY['weave_actors','weave_ontologies','weave_ontology_versions','weave_ontology_active_versions','weave_ontology_releases'] LOOP
  EXECUTE format('DROP TRIGGER guard_draft_global_input ON %I',tab);
 END LOOP;
END $$;
-- +goose StatementEnd
DROP TABLE weave_draft_effects,weave_draft_mutations,weave_draft_projects,weave_draft_snapshots,weave_draft_blobs,weave_draft_row_owners,weave_draft_project_fences;
DROP FUNCTION weave_lock_entity_in_transaction(text,text,bigint),weave_guard_draft_fences(),weave_lock_draft_capture(text),weave_draft_fence(text),weave_guard_draft_state(),weave_guard_draft_owners(),weave_guard_draft_effect_insert(),weave_check_draft_receipt_closed(),weave_guard_draft_receipt(),weave_guard_draft_immutable(),weave_store_draft_snapshot(jsonb),weave_guard_draft_global_input(),weave_capture_draft_effect(),weave_guard_draft_write(),weave_draft_owner(text,jsonb),weave_draft_row_image(anyelement,regclass),weave_draft_lock_key(text),weave_draft_sections_v1();
