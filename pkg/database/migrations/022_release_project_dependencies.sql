-- +goose Up
-- This marker certifies project-owned entity dependency resolution only.
-- Ontology artifacts and actor descriptors have separate
-- lifecycles; never interpret this as whole-output snapshot completeness.
ALTER TABLE weave_releases ADD COLUMN project_dependencies_version smallint NOT NULL DEFAULT 0
    CHECK (project_dependencies_version IN (0, 1));

CREATE TABLE weave_release_project_dependencies (
    project_id text NOT NULL,
    version_number text NOT NULL,
    source_project_id text NOT NULL,
    source_version text NOT NULL CHECK (btrim(source_version) <> ''),
    PRIMARY KEY (project_id, version_number, source_project_id),
    UNIQUE (project_id, version_number, source_project_id, source_version),
    CHECK (project_id <> source_project_id),
    FOREIGN KEY (project_id, version_number)
        REFERENCES weave_releases (project_id, version) ON DELETE CASCADE,
    -- A referenced release (and therefore its project) cannot be deleted.
    -- Retirement via archived_at remains possible and does not break a pin.
    FOREIGN KEY (source_project_id, source_version)
        REFERENCES weave_releases (project_id, version) ON DELETE NO ACTION
);
CREATE INDEX weave_release_dependencies_source_idx
    ON weave_release_project_dependencies (source_project_id, source_version);

-- Direct foreign entity membership survives deletion from the source draft.
-- Transitive project pins live above; child readers use their own captured
-- entity membership. One source selection per entity is intentional.
CREATE TABLE weave_release_dependency_entities (
    project_id text NOT NULL,
    version_number text NOT NULL,
    entity_type text NOT NULL,
    entity_id text NOT NULL,
    source_project_id text NOT NULL,
    source_version text NOT NULL,
    PRIMARY KEY (project_id, version_number, entity_type, entity_id),
    FOREIGN KEY (project_id, version_number, source_project_id, source_version)
        REFERENCES weave_release_project_dependencies
        (project_id, version_number, source_project_id, source_version) ON DELETE CASCADE
);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM weave_releases WHERE project_dependencies_version = 1) THEN
        RAISE EXCEPTION 'cannot downgrade captured release project dependencies';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE weave_release_dependency_entities;
DROP TABLE weave_release_project_dependencies;
ALTER TABLE weave_releases DROP COLUMN project_dependencies_version;
