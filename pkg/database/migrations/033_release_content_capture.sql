-- +goose Up
-- Contract 1 certifies the 22 owned sections, not serializer/restore coverage.
-- Existing history stays unverified; only this transaction's new cut can seal.
ALTER TABLE weave_releases
    ADD COLUMN content_capture_version smallint NOT NULL DEFAULT 0 CHECK (content_capture_version IN (0,1)),
    ADD COLUMN content_capture_receipt jsonb,
    ADD COLUMN content_capture_xid xid8;

-- Parent identities must coordinate even when no parent is visible in an old
-- snapshot. Retain keys across deletion/recreation, just like project fences.
CREATE TABLE weave_release_content_fences (
    archive_table text NOT NULL,
    entity_id text NOT NULL,
    version_number text NOT NULL,
    PRIMARY KEY(archive_table,entity_id,version_number)
);
-- +goose StatementBegin
CREATE FUNCTION weave_lock_content_parent(tab text, id text, version text) RETURNS void LANGUAGE sql AS $$
    INSERT INTO weave_release_content_fences VALUES (tab,id,version)
    ON CONFLICT(archive_table,entity_id,version_number) DO UPDATE SET entity_id=EXCLUDED.entity_id;
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_guard_content_fences() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='UPDATE' AND to_jsonb(NEW)=to_jsonb(OLD) THEN RETURN NEW; END IF;
    RAISE EXCEPTION 'release content coordination keys cannot be removed or retargeted' USING ERRCODE='23514';
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_content_fences BEFORE UPDATE OR DELETE ON weave_release_content_fences
FOR EACH ROW EXECUTE FUNCTION weave_guard_content_fences();
CREATE TRIGGER guard_content_fences_truncate BEFORE TRUNCATE ON weave_release_content_fences
FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_content_fences();

-- Reviewed contract: field expressions and selectors are checked against the
-- independent capture ledger. No user-supplied SQL or table names enter it.
-- +goose StatementBegin
CREATE FUNCTION weave_content_sections_v1() RETURNS TABLE(
    archive_table text, owner_column text, parent_table text, parent_column text,
    parent_identity boolean, expected_sql text
) LANGUAGE sql IMMUTABLE AS $$
VALUES
    ('weave_adoptions_archive','project_id','','',false,
     $query$SELECT DISTINCT ON (a.context_entity_type, a.context_entity_id, a.entity_type, a.source_project_id, a.source_entity_id) a.project_id AS "project_id", a.context_entity_type AS "context_entity_type", a.context_entity_id AS "context_entity_id", a.entity_type AS "entity_type", a.source_project_id AS "source_project_id", a.source_entity_id AS "source_entity_id", COALESCE(d.source_version, a.source_version) AS "source_version", a.adopted_at AS "adopted_at", a.created_by_id AS "created_by_id", $2::text AS "version_number" FROM weave_adoptions a LEFT JOIN weave_release_project_dependencies d ON d.project_id = a.project_id AND d.version_number = $2::text AND d.source_project_id = a.source_project_id WHERE a.project_id = $1::text ORDER BY a.context_entity_type, a.context_entity_id, a.entity_type, a.source_project_id, a.source_entity_id, a.adopted_at, a.id$query$),
    ('weave_categories_archive','project_id','','',false,
     $query$SELECT id AS "id", created_at AS "created_at", updated_at AS "updated_at", semantic_id AS "semantic_id", system_name AS "system_name", ui_name AS "ui_name", description AS "description", CASE WHEN status = 'deprecated' THEN status ELSE 'published' END AS "status", project_id AS "project_id", canonical_order AS "canonical_order", deprecated AS "deprecated", $2::text AS "version_number" FROM weave_categories WHERE project_id = $1$query$),
    ('weave_collection_placements_archive','project_id','','',false,
     $query$SELECT id AS "id", project_id AS "project_id", model_id AS "model_id", category_id AS "category_id", collection_id AS "collection_id", is_required AS "is_required", min_occurs AS "min_occurs", max_occurs AS "max_occurs", is_hidden AS "is_hidden", created_at AS "created_at", updated_at AS "updated_at", $2::text AS "version_number" FROM weave_collection_placements WHERE project_id = $1$query$),
    ('weave_collections_archive','project_id','','',false,
     $query$SELECT id AS "id", created_at AS "created_at", updated_at AS "updated_at", system_name AS "system_name", ui_name AS "ui_name", description AS "description", CASE WHEN status = 'deprecated' THEN status ELSE 'published' END AS "status", project_id AS "project_id", ontology_scope AS "ontology_scope", collection_number AS "collection_number", canonical_collection_order AS "canonical_collection_order", staging_id AS "staging_id", deprecated AS "deprecated", default_category_id AS "default_category_id", $2::text AS "version_number" FROM weave_collections WHERE project_id = $1$query$),
    ('weave_concept_broader_archive','','weave_concept_lists_archive','scheme_id',false,
     $query$SELECT b.id AS "id", b.concept_id AS "concept_id", b.broader_id AS "broader_id", b.scheme_id AS "scheme_id", b.position AS "position", $2::text AS "version_number", b.created_at AS "created_at" FROM weave_concept_broader b JOIN weave_concept_lists cl ON cl.id = b.scheme_id WHERE cl.project_id = $1::text$query$),
    ('weave_concept_list_entries_archive','','weave_concept_lists_archive','concept_list_id',false,
     $query$SELECT e.id AS "id", e.concept_list_id AS "concept_list_id", e.vocabulary_entry_id AS "vocabulary_entry_id", e.position AS "position", e.custom_label AS "custom_label", e.created_at AS "created_at", e.updated_at AS "updated_at", $2::text AS "version_number" FROM weave_concept_list_entries e JOIN weave_concept_lists cl ON cl.id = e.concept_list_id WHERE cl.project_id = $1$query$),
    ('weave_concept_lists_archive','project_id','','',true,
     $query$SELECT id AS "id", created_at AS "created_at", updated_at AS "updated_at", semantic_id AS "semantic_id", system_name AS "system_name", ui_name AS "ui_name", description AS "description", CASE WHEN status = 'deprecated' THEN status ELSE 'published' END AS "status", project_id AS "project_id", list_type AS "list_type", vocabulary_id AS "vocabulary_id", is_closed AS "is_closed", $2::text AS "version_number" FROM weave_concept_lists WHERE project_id = $1$query$),
    ('weave_entity_forks_archive','project_id','','',false,
     $query$SELECT project_id AS "project_id", entity_type AS "entity_type", fork_entity_id AS "fork_entity_id", source_project_id AS "source_project_id", source_entity_id AS "source_entity_id", source_version AS "source_version", forked_at AS "forked_at", created_by_id AS "created_by_id", $2::text AS "version_number" FROM weave_entity_forks WHERE project_id = $1$query$),
    ('weave_example_values_archive','','weave_examples_archive','example_id',false,
     $query$SELECT v.id AS "id", v.example_id AS "example_id", v.override_id AS "override_id", v.field_id AS "field_id", v.part_of_collection_id AS "part_of_collection_id", v.occurrence_index AS "occurrence_index", v.value_kind AS "value_kind", v.value_payload AS "value_payload", v.text_value AS "text_value", v.number_value AS "number_value", v.date_value AS "date_value", v.uri_value AS "uri_value", v.concept_uri AS "concept_uri", v.linked_example_id AS "linked_example_id", v.slot_path AS "slot_path", v.created_at AS "created_at", v.updated_at AS "updated_at", $2::text AS "version_number" FROM weave_example_values v JOIN weave_examples e ON e.id = v.example_id WHERE e.project_id = $1$query$),
    ('weave_examples_archive','project_id','','',true,
     $query$SELECT id AS "id", project_id AS "project_id", entity_type AS "entity_type", entity_id AS "entity_id", title AS "title", description AS "description", status AS "status", created_at AS "created_at", updated_at AS "updated_at", $2::text AS "version_number" FROM weave_examples WHERE project_id = $1$query$),
    ('weave_field_overrides_archive','project_id','','',true,
     $query$SELECT id AS "id", field_id AS "field_id", project_id AS "project_id", entity_type AS "entity_type", entity_id AS "entity_id", position AS "position", collection_order AS "collection_order", display_name AS "display_name", description AS "description", collection_name AS "collection_name", category_id AS "category_id", part_of_collection_id AS "part_of_collection_id", expected_value_type AS "expected_value_type", set_value AS "set_value", is_required AS "is_required", min_occurs AS "min_occurs", max_occurs AS "max_occurs", is_hidden AS "is_hidden", visibility AS "visibility", staging_id AS "staging_id", created_at AS "created_at", updated_at AS "updated_at", content_hash AS "content_hash", $2::text AS "version_number", set_value_entry_id AS "set_value_entry_id" FROM weave_field_overrides WHERE project_id = $1::text$query$),
    ('weave_fields_archive','project_id','','',false,
     $query$SELECT id AS "id", created_at AS "created_at", updated_at AS "updated_at", semantic_id AS "semantic_id", system_name AS "system_name", ui_name AS "ui_name", description AS "description", CASE WHEN status = 'deprecated' THEN status ELSE 'published' END AS "status", project_id AS "project_id", ontology_scope AS "ontology_scope", ontology_path AS "ontology_path", path_elements AS "path_elements", expected_value_type AS "expected_value_type", examples AS "examples", staging_id AS "staging_id", deprecated AS "deprecated", $2::text AS "version_number", subfield_paths AS "subfield_paths" FROM weave_fields WHERE project_id = $1$query$),
    ('weave_models_archive','project_id','','',false,
     $query$SELECT id AS "id", created_at AS "created_at", updated_at AS "updated_at", system_name AS "system_name", ui_name AS "ui_name", description AS "description", CASE WHEN status = 'deprecated' THEN status ELSE 'published' END AS "status", project_id AS "project_id", ontology_scope AS "ontology_scope", staging_id AS "staging_id", deprecated AS "deprecated", $2::text AS "version_number", model_type AS "model_type" FROM weave_models WHERE project_id = $1::text$query$),
    ('weave_release_namespace_bindings_archive','release_project_id','','',false,
     $query$SELECT $1::text AS "release_project_id", $2::text AS "version_number", id AS "id", created_at AS "created_at", updated_at AS "updated_at", airtable_raw AS "airtable_raw", airtable_context AS "airtable_context", airtable_ref AS "airtable_ref", semantic_id AS "semantic_id", long_semantic_id AS "long_semantic_id", system_name AS "system_name", ui_name AS "ui_name", description AS "description", uri AS "uri", project_id AS "project_id", prefix AS "prefix", namespace AS "namespace", weight AS "weight", source AS "source", ontology_id AS "ontology_id" FROM weave_namespace_bindings WHERE project_id = $1::text OR project_id IS NULL OR project_id = ''$query$),
    ('weave_override_refs_archive','project_id','weave_field_overrides_archive','override_id',false,
     $query$SELECT r.override_id AS "override_id", r.ref_type AS "ref_type", r.target_id AS "target_id", r.semantic_id AS "semantic_id", r.position AS "position", o.project_id AS "project_id", $2::text AS "version_number" FROM weave_override_refs r JOIN weave_field_overrides o ON o.id = r.override_id WHERE o.project_id = $1$query$),
    ('weave_project_actors_archive','project_id','','',false,
     $query$SELECT project_id AS "project_id", actor_id AS "actor_id", role AS "role", created_at AS "created_at", $2::text AS "version_number" FROM weave_project_actors WHERE project_id = $1$query$),
    ('weave_project_attributions_archive','project_id','','',false,
     $query$SELECT project_id AS "project_id", actor_id AS "actor_id", kind AS "kind", "position" AS "position", note AS "note", created_at AS "created_at", $2::text AS "version_number" FROM weave_project_attributions WHERE project_id = $1$query$),
    ('weave_project_inheritance_archive','project_id','','',false,
     $query$SELECT project_id AS "project_id", parent_project_id AS "parent_project_id", is_primary AS "is_primary", canonical_order AS "canonical_order", adopted_at AS "adopted_at", source_mode AS "source_mode", source_version AS "source_version", $2::text AS "version_number" FROM weave_project_inheritance i WHERE i.project_id = $1::text$query$),
    ('weave_project_ontology_versions_archive','project_id','','',false,
     $query$SELECT project_id AS "project_id", ontology_version_id AS "ontology_version_id", added_at AS "added_at", added_by_id AS "added_by_id", is_primary AS "is_primary", usage_notes AS "usage_notes", $2::text AS "version_number" FROM weave_project_ontology_versions o WHERE o.project_id = $1::text$query$),
    ('weave_projects_archive','id','','',false,
     $query$SELECT id AS "id", created_at AS "created_at", updated_at AS "updated_at", system_name AS "system_name", ui_name AS "ui_name", description AS "description", CASE WHEN status = 'deprecated' THEN status ELSE 'published' END AS "status", namespace AS "namespace", parent_project_id AS "parent_project_id", staging_id AS "staging_id", owner_id AS "owner_id", visibility AS "visibility", deprecated AS "deprecated", license AS "license", readme AS "readme", topics AS "topics", base_url AS "base_url", created_by_id AS "created_by_id", $2::text AS "version_number", concept_namespace AS "concept_namespace", weave_effective_concept_namespace_v1(concept_namespace) AS "effective_concept_namespace", enforce_concept_lists AS "enforce_concept_lists" FROM weave_projects p WHERE p.id = $1::text$query$),
    ('weave_vocabularies_archive','project_id','','',true,
     $query$SELECT id AS "id", project_id AS "project_id", semantic_id AS "semantic_id", system_name AS "system_name", ui_name AS "ui_name", description AS "description", CASE WHEN status = 'deprecated' THEN status ELSE 'published' END AS "status", connector_type AS "connector_type", base_uri AS "base_uri", config AS "config", deprecated AS "deprecated", created_at AS "created_at", updated_at AS "updated_at", $2::text AS "version_number" FROM weave_vocabularies WHERE project_id = $1$query$),
    ('weave_vocabulary_entries_archive','','weave_vocabularies_archive','vocabulary_id',false,
     $query$SELECT e.id AS "id", e.vocabulary_id AS "vocabulary_id", e.uri AS "uri", e.label AS "label", e.scope_note AS "scope_note", e.broader_uri AS "broader_uri", e.broader_path AS "broader_path", e.broader_path_items AS "broader_path_items", e.external_id AS "external_id", e.created_at AS "created_at", e.updated_at AS "updated_at", $2::text AS "version_number" FROM weave_vocabulary_entries e JOIN weave_vocabularies v ON v.id = e.vocabulary_id WHERE v.project_id = $1$query$);
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION weave_content_archive_sql_v1(tab text, owner_col text, parent_tab text, parent_col text)
RETURNS text LANGUAGE plpgsql IMMUTABLE AS $$
BEGIN
    IF parent_tab<>'' AND owner_col='' THEN
        RETURN format('SELECT to_jsonb(a) AS body FROM %I a JOIN %I p ON p.id::text=a.%I::text AND p.version_number=a.version_number WHERE p.project_id=$1 AND a.version_number=$2',tab,parent_tab,parent_col);
    END IF;
    RETURN format('SELECT to_jsonb(a) AS body FROM %I a WHERE a.%I=$1 AND a.version_number=$2',tab,owner_col);
END $$;
-- +goose StatementEnd

-- Covers owned rows AND the five separately validated dependency/evidence
-- tables. The parent mapping is fixed at migration time in trigger arguments.
-- +goose StatementBegin
CREATE FUNCTION weave_guard_content_rows() RETURNS trigger LANGUAGE plpgsql AS $$
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
DO $$ DECLARE s record; tab text;
BEGIN
    FOR s IN SELECT * FROM weave_content_sections_v1() LOOP
        EXECUTE format('CREATE TRIGGER guard_content_rows BEFORE INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION weave_guard_content_rows(%L,%L,%L,%L)',
            s.archive_table,s.owner_column,s.parent_table,s.parent_column,s.parent_identity::text);
        EXECUTE format('CREATE TRIGGER guard_content_truncate BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_content_rows()',s.archive_table);
    END LOOP;
    FOREACH tab IN ARRAY ARRAY['weave_release_project_dependencies','weave_release_dependency_entities','weave_release_ontology_dependencies','weave_release_descriptors','weave_release_ontology_references'] LOOP
        EXECUTE format('CREATE TRIGGER guard_content_rows BEFORE INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION weave_guard_content_rows(''project_id'','''','''',''false'')',tab);
        EXECUTE format('CREATE TRIGGER guard_content_truncate BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_content_rows()',tab);
    END LOOP;
END $$;
-- +goose StatementEnd

-- Finalization compares full multisets, not just counts or presence. It runs in
-- the release's RR transaction after all captures and outbox insertion. Receipts
-- include empty sections; their hashes are PostgreSQL evidence, not Git encoding.
-- +goose StatementBegin
CREATE FUNCTION weave_finish_content_capture() RETURNS trigger LANGUAGE plpgsql
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
CREATE TRIGGER finish_content_capture BEFORE INSERT OR UPDATE OR DELETE ON weave_releases
FOR EACH ROW EXECUTE FUNCTION weave_finish_content_capture();

-- +goose Down
-- +goose StatementBegin
DO $$ DECLARE s record; tab text;
BEGIN
    IF EXISTS(SELECT 1 FROM weave_releases WHERE content_capture_version=1) THEN
        RAISE EXCEPTION 'cannot downgrade captured release content';
    END IF;
    FOR s IN SELECT * FROM weave_content_sections_v1() LOOP
        EXECUTE format('DROP TRIGGER guard_content_rows ON %I',s.archive_table);
        EXECUTE format('DROP TRIGGER guard_content_truncate ON %I',s.archive_table);
    END LOOP;
    FOREACH tab IN ARRAY ARRAY['weave_release_project_dependencies','weave_release_dependency_entities','weave_release_ontology_dependencies','weave_release_descriptors','weave_release_ontology_references'] LOOP
        EXECUTE format('DROP TRIGGER guard_content_rows ON %I',tab);
        EXECUTE format('DROP TRIGGER guard_content_truncate ON %I',tab);
    END LOOP;
END $$;
-- +goose StatementEnd
DROP TRIGGER finish_content_capture ON weave_releases;
DROP FUNCTION weave_finish_content_capture(),weave_guard_content_rows();
DROP FUNCTION weave_content_archive_sql_v1(text,text,text,text),weave_content_sections_v1();
DROP FUNCTION weave_lock_content_parent(text,text,text);
DROP TABLE weave_release_content_fences;
DROP FUNCTION weave_guard_content_fences();
ALTER TABLE weave_releases DROP COLUMN content_capture_version, DROP COLUMN content_capture_receipt, DROP COLUMN content_capture_xid;
