-- +goose Up
-- Version 1 certifies only project concept namespace and model types, not a
-- complete project snapshot. Historical releases deliberately remain at zero.
ALTER TABLE weave_releases ADD COLUMN metadata_capture_version smallint NOT NULL DEFAULT 0
    CHECK (metadata_capture_version IN (0, 1));
ALTER TABLE weave_projects_archive ADD COLUMN concept_namespace text;
ALTER TABLE weave_projects_archive ADD COLUMN effective_concept_namespace text;

-- A catalog row may not yet exist in a concurrent writer's RR snapshot. Touch
-- this persistent key before BOTH creating a release and changing its archives:
-- ON CONFLICT UPDATE forces a stale snapshot to abort, even across release
-- deletion/recreation. These small coordination rows are not released content.
CREATE TABLE weave_release_metadata_fences (
    project_id text NOT NULL,
    version_number text NOT NULL,
    PRIMARY KEY (project_id, version_number)
);
-- +goose StatementBegin
CREATE FUNCTION weave_lock_release_metadata(project text, version text) RETURNS void LANGUAGE sql AS $$
    INSERT INTO weave_release_metadata_fences(project_id,version_number) VALUES (project,version)
    ON CONFLICT (project_id,version_number) DO UPDATE SET project_id=EXCLUDED.project_id;
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_guard_metadata_fences() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='UPDATE' THEN
        IF NEW.project_id=OLD.project_id AND NEW.version_number=OLD.version_number THEN RETURN NEW; END IF;
    END IF;
    RAISE EXCEPTION 'release metadata coordination keys cannot be removed or retargeted' USING ERRCODE='23514';
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_metadata_fences BEFORE UPDATE OR DELETE ON weave_release_metadata_fences
FOR EACH ROW EXECUTE FUNCTION weave_guard_metadata_fences();
CREATE TRIGGER guard_metadata_fences_truncate BEFORE TRUNCATE ON weave_release_metadata_fences
FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_metadata_fences();

-- Preserve the existing strings.TrimSpace policy, including Unicode whitespace.
-- Keep this versioned function stable: captured defaults are historical values.
-- +goose StatementBegin
CREATE FUNCTION weave_effective_concept_namespace_v1(value text) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT COALESCE(NULLIF(btrim(value,
        U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'), ''),
        'https://vocab.pletka.io/');
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION weave_guard_metadata_capture() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER guard_metadata_capture BEFORE INSERT OR UPDATE ON weave_releases
FOR EACH ROW EXECUTE FUNCTION weave_guard_metadata_capture();

-- Protect the rows carrying certified metadata, including insert/key-move and
-- truncate bypasses. Lock the persistent key and the release row to serialize
-- with finalization, including when a concurrent release row is not visible.
-- Deleting the owning release permits the normal archive cleanup afterward.
-- +goose StatementBegin
CREATE FUNCTION weave_guard_captured_metadata_rows() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    body jsonb;
    project text;
    captured smallint;
BEGIN
    IF TG_OP = 'TRUNCATE' THEN
        -- An old RR snapshot could miss a newly certified release. TRUNCATE is
        -- not MVCC-safe; use release deletion followed by ordinary row cleanup.
        RAISE EXCEPTION 'cannot truncate release metadata archives' USING ERRCODE='23514';
    END IF;
    FOR body IN SELECT value FROM jsonb_array_elements(
        CASE TG_OP WHEN 'INSERT' THEN jsonb_build_array(to_jsonb(NEW))
                   WHEN 'DELETE' THEN jsonb_build_array(to_jsonb(OLD))
                   ELSE jsonb_build_array(to_jsonb(OLD),to_jsonb(NEW)) END)
    LOOP
        project := CASE WHEN TG_TABLE_NAME='weave_projects_archive' THEN body->>'id' ELSE body->>'project_id' END;
        PERFORM weave_lock_release_metadata(project,body->>'version_number');
        SELECT metadata_capture_version INTO captured FROM weave_releases
        WHERE project_id=project AND version=body->>'version_number' FOR SHARE;
        IF captured=1 THEN
            RAISE EXCEPTION 'captured release metadata is immutable' USING ERRCODE='23514';
        END IF;
    END LOOP;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_project_metadata_rows BEFORE INSERT OR UPDATE OR DELETE ON weave_projects_archive
FOR EACH ROW EXECUTE FUNCTION weave_guard_captured_metadata_rows();
CREATE TRIGGER guard_project_metadata_truncate BEFORE TRUNCATE ON weave_projects_archive
FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_captured_metadata_rows();
CREATE TRIGGER guard_model_metadata_rows BEFORE INSERT OR UPDATE OR DELETE ON weave_models_archive
FOR EACH ROW EXECUTE FUNCTION weave_guard_captured_metadata_rows();
CREATE TRIGGER guard_model_metadata_truncate BEFORE TRUNCATE ON weave_models_archive
FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_captured_metadata_rows();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM weave_releases WHERE metadata_capture_version=1) THEN
        RAISE EXCEPTION 'cannot downgrade captured release metadata';
    END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER guard_metadata_capture ON weave_releases;
DROP TRIGGER guard_project_metadata_rows ON weave_projects_archive;
DROP TRIGGER guard_project_metadata_truncate ON weave_projects_archive;
DROP TRIGGER guard_model_metadata_rows ON weave_models_archive;
DROP TRIGGER guard_model_metadata_truncate ON weave_models_archive;
DROP FUNCTION weave_guard_metadata_capture();
DROP FUNCTION weave_guard_captured_metadata_rows();
DROP FUNCTION weave_lock_release_metadata(text,text);
DROP TABLE weave_release_metadata_fences;
DROP FUNCTION weave_guard_metadata_fences();
DROP FUNCTION weave_effective_concept_namespace_v1(text);
ALTER TABLE weave_projects_archive DROP COLUMN concept_namespace, DROP COLUMN effective_concept_namespace;
ALTER TABLE weave_releases DROP COLUMN metadata_capture_version;
