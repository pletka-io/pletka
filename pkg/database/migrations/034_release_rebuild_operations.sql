-- +goose Up
-- One explicit maintenance operation owns the database/Git publication gap.
CREATE TABLE weave_release_rebuild_operations (
    id text PRIMARY KEY,
    document text NOT NULL,
    result jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);
CREATE UNIQUE INDEX weave_release_rebuild_one_active ON weave_release_rebuild_operations ((true)) WHERE completed_at IS NULL;

-- +goose StatementBegin
CREATE FUNCTION weave_release_rebuild_fingerprint(projects text[]) RETURNS text LANGUAGE plpgsql
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

-- Deleting children before parents preserves archive ownership for cleanup.
-- Caller first removes the selected releases, in reverse dependency order.
-- +goose StatementBegin
CREATE FUNCTION weave_clear_release_for_rebuild(project text, release_version text) RETURNS void LANGUAGE plpgsql AS $$
DECLARE s record;
BEGIN
    IF EXISTS(SELECT 1 FROM weave_releases r WHERE r.project_id=project AND r.version=release_version) THEN
        RAISE EXCEPTION 'remove owning release before rebuild cleanup';
    END IF;
    FOR s IN SELECT * FROM weave_content_sections_v1() ORDER BY (parent_table<>'') DESC,archive_table LOOP
        IF s.parent_table<>'' AND s.owner_column='' THEN
            EXECUTE format('DELETE FROM %I a USING %I p WHERE p.id::text=a.%I::text AND p.version_number=a.version_number AND p.project_id=$1 AND a.version_number=$2',s.archive_table,s.parent_table,s.parent_column) USING project,release_version;
        ELSE
            EXECUTE format('DELETE FROM %I WHERE %I=$1 AND version_number=$2',s.archive_table,s.owner_column) USING project,release_version;
        END IF;
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM weave_release_rebuild_operations) THEN
        RAISE EXCEPTION 'cannot discard release rebuild operation records';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE weave_release_rebuild_operations;
DROP FUNCTION weave_release_rebuild_fingerprint(text[]),weave_clear_release_for_rebuild(text,text);
