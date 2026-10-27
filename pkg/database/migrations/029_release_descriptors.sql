-- +goose Up
-- Historical releases deliberately have no descriptor row. Never infer them
-- from current actor data or the host configuration of a later materializer.
-- One inventory drives both capture and storage validation.
CREATE VIEW weave_release_actor_references AS
SELECT id AS project_id, version_number, owner_id AS actor_id FROM weave_projects_archive
UNION SELECT id, version_number, created_by_id FROM weave_projects_archive
UNION SELECT project_id, version, created_by_id FROM weave_releases
UNION SELECT project_id, version_number, created_by_id FROM weave_adoptions_archive
UNION SELECT project_id, version_number, created_by_id FROM weave_entity_forks_archive
UNION SELECT project_id, version_number, actor_id FROM weave_project_attributions_archive
UNION SELECT project_id, version_number, actor_id FROM weave_project_actors_archive;

CREATE TABLE weave_release_descriptors (
    project_id text NOT NULL,
    version_number text NOT NULL,
    document jsonb NOT NULL,
    PRIMARY KEY (project_id, version_number),
    FOREIGN KEY (project_id, version_number) REFERENCES weave_releases(project_id, version) ON DELETE CASCADE,
    CHECK ((document->>'format' = '1') IS TRUE),
    CHECK ((document->>'project_id' = project_id) IS TRUE),
    CHECK ((document->>'version' = version_number) IS TRUE),
    CHECK ((jsonb_typeof(document->'actors') = 'object') IS TRUE),
    CHECK ((jsonb_typeof(document->'project_modules') = 'object') IS TRUE),
    CHECK ((jsonb_typeof(document->'ontology_modules') = 'object') IS TRUE),
    CHECK ((length(document->'project_modules'->>project_id) > 0) IS TRUE)
);
-- +goose StatementBegin
CREATE FUNCTION weave_guard_release_descriptors() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER guard_release_descriptors BEFORE INSERT OR UPDATE OR DELETE ON weave_release_descriptors FOR EACH ROW EXECUTE FUNCTION weave_guard_release_descriptors();
CREATE TRIGGER guard_release_descriptors_truncate BEFORE TRUNCATE ON weave_release_descriptors FOR EACH STATEMENT EXECUTE FUNCTION weave_guard_release_descriptors();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM weave_release_descriptors) THEN
        RAISE EXCEPTION 'cannot downgrade captured release descriptors';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE weave_release_descriptors;
DROP FUNCTION weave_guard_release_descriptors();
DROP VIEW weave_release_actor_references;
