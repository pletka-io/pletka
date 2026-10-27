-- +goose Up
-- Historical links alone do not prove what ontology bytes a release saw.
ALTER TABLE weave_releases ADD COLUMN ontology_dependencies_version smallint NOT NULL DEFAULT 0
    CHECK (ontology_dependencies_version IN (0,1));
CREATE TABLE weave_release_ontology_dependencies (
    project_id text NOT NULL,
    version_number text NOT NULL,
    ontology_id text NOT NULL,
    ontology_version_id text NOT NULL,
    digest text NOT NULL,
    PRIMARY KEY(project_id,version_number,ontology_id),
    FOREIGN KEY(project_id,version_number) REFERENCES weave_releases(project_id,version) ON DELETE CASCADE,
    FOREIGN KEY(ontology_id,ontology_version_id,digest)
        REFERENCES weave_ontology_releases(ontology_id,version_id,digest) ON DELETE RESTRICT
);
CREATE INDEX weave_release_ontology_source ON weave_release_ontology_dependencies(ontology_version_id);
-- +goose StatementBegin
CREATE FUNCTION weave_guard_project_ontology_pins() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'TRUNCATE' THEN
        IF EXISTS (SELECT 1 FROM weave_releases WHERE ontology_dependencies_version=1) THEN
            RAISE EXCEPTION 'cannot truncate captured ontology pins' USING ERRCODE='23514';
        END IF;
        RETURN NULL;
    END IF;
    IF TG_OP <> 'DELETE' AND EXISTS (SELECT 1 FROM weave_releases WHERE project_id=NEW.project_id AND version=NEW.version_number AND ontology_dependencies_version=1) THEN
        RAISE EXCEPTION 'captured ontology pins are immutable' USING ERRCODE='23514';
    END IF;
    IF TG_OP <> 'INSERT' AND EXISTS (SELECT 1 FROM weave_releases WHERE project_id=OLD.project_id AND version=OLD.version_number AND ontology_dependencies_version=1) THEN
        RAISE EXCEPTION 'captured ontology pins are immutable' USING ERRCODE='23514';
    END IF;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER guard_project_ontology_pins BEFORE INSERT OR UPDATE OR DELETE ON weave_release_ontology_dependencies FOR EACH ROW EXECUTE FUNCTION weave_guard_project_ontology_pins();
CREATE TRIGGER guard_project_ontology_pins_truncate BEFORE TRUNCATE ON weave_release_ontology_dependencies FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_project_ontology_pins();
-- +goose StatementBegin
CREATE FUNCTION weave_finish_project_ontology_pins() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='INSERT' THEN
        IF NEW.ontology_dependencies_version<>0 THEN
            RAISE EXCEPTION 'ontology inventory must be captured after release insertion' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END IF;
    IF OLD.ontology_dependencies_version=1 AND NEW.ontology_dependencies_version<>1 THEN
        RAISE EXCEPTION 'cannot unset captured ontology inventory' USING ERRCODE='23514';
    END IF;
    IF OLD.ontology_dependencies_version=0 AND NEW.ontology_dependencies_version=1 THEN
        IF current_setting('transaction_isolation') NOT IN ('repeatable read','serializable') THEN
            RAISE EXCEPTION 'ontology inventory requires repeatable read' USING ERRCODE='23514';
        END IF;
        IF EXISTS (SELECT 1 FROM weave_project_ontology_versions_archive l WHERE l.project_id=NEW.project_id AND l.version_number=NEW.version
            AND NOT EXISTS (SELECT 1 FROM weave_release_ontology_dependencies p WHERE p.project_id=NEW.project_id AND p.version_number=NEW.version AND p.ontology_version_id=l.ontology_version_id)) THEN
            RAISE EXCEPTION 'ontology inventory omits a declared link' USING ERRCODE='23514';
        END IF;
		IF EXISTS (
			WITH bodies AS (
				SELECT to_jsonb(f) AS body FROM weave_fields_archive f WHERE project_id=NEW.project_id AND version_number=NEW.version
				UNION ALL SELECT to_jsonb(m) FROM weave_models_archive m WHERE project_id=NEW.project_id AND version_number=NEW.version
				UNION ALL SELECT to_jsonb(c) FROM weave_collections_archive c WHERE project_id=NEW.project_id AND version_number=NEW.version
			)
			SELECT 1 FROM bodies, LATERAL jsonb_path_query(body,'$.**.ontology_version_id') AS value
			WHERE value #>> '{}' IS NOT NULL AND btrim(value #>> '{}')<>'' AND NOT EXISTS (
				SELECT 1 FROM weave_release_ontology_dependencies p WHERE p.project_id=NEW.project_id AND p.version_number=NEW.version AND p.ontology_version_id=value #>> '{}')
		) THEN
			RAISE EXCEPTION 'ontology inventory omits an explicit version selection' USING ERRCODE='23514';
		END IF;
        IF EXISTS (SELECT 1 FROM weave_release_ontology_dependencies p
            JOIN weave_ontology_release_dependencies d ON d.consumer_version_id=p.ontology_version_id
            WHERE p.project_id=NEW.project_id AND p.version_number=NEW.version AND NOT EXISTS (
                SELECT 1 FROM weave_release_ontology_dependencies actual WHERE actual.project_id=NEW.project_id AND actual.version_number=NEW.version
                    AND (actual.ontology_id,actual.ontology_version_id,actual.digest)=(d.ontology_id,d.version_id,d.digest))) THEN
            RAISE EXCEPTION 'ontology inventory omits a source closure' USING ERRCODE='23514';
        END IF;
        IF EXISTS (SELECT 1 FROM weave_release_project_dependencies d JOIN weave_releases r ON r.project_id=d.source_project_id AND r.version=d.source_version
            WHERE d.project_id=NEW.project_id AND d.version_number=NEW.version AND r.ontology_dependencies_version<>1) THEN
            RAISE EXCEPTION 'source project ontology inventory is unverified' USING ERRCODE='23514';
        END IF;
        IF EXISTS (SELECT 1 FROM weave_release_project_dependencies d JOIN weave_release_ontology_dependencies p ON p.project_id=d.source_project_id AND p.version_number=d.source_version
            WHERE d.project_id=NEW.project_id AND d.version_number=NEW.version AND NOT EXISTS (
                SELECT 1 FROM weave_release_ontology_dependencies actual WHERE actual.project_id=NEW.project_id AND actual.version_number=NEW.version
                    AND (actual.ontology_id,actual.ontology_version_id,actual.digest)=(p.ontology_id,p.ontology_version_id,p.digest))) THEN
            RAISE EXCEPTION 'ontology inventory omits a source project pin' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER finish_project_ontology_pins BEFORE INSERT OR UPDATE ON weave_releases FOR EACH ROW EXECUTE FUNCTION weave_finish_project_ontology_pins();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM weave_releases WHERE ontology_dependencies_version=1) THEN
        RAISE EXCEPTION 'cannot downgrade captured ontology dependencies';
    END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER finish_project_ontology_pins ON weave_releases;
DROP TABLE weave_release_ontology_dependencies;
DROP FUNCTION weave_guard_project_ontology_pins(),weave_finish_project_ontology_pins();
ALTER TABLE weave_releases DROP COLUMN ontology_dependencies_version;
