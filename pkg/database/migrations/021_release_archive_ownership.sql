-- +goose Up
ALTER TABLE weave_releases
    ADD COLUMN archive_ownership_version smallint NOT NULL DEFAULT 0
    CHECK (archive_ownership_version IN (0, 1));

CREATE TABLE weave_release_namespace_bindings_archive (
    release_project_id text NOT NULL,
    version_number text NOT NULL,
    id text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    airtable_raw jsonb,
    airtable_context jsonb,
    airtable_ref text,
    semantic_id text,
    long_semantic_id text,
    system_name text,
    ui_name jsonb,
    description jsonb,
    uri text,
    project_id text,
    prefix text NOT NULL,
    namespace text NOT NULL,
    weight bigint NOT NULL,
    source text NOT NULL,
    ontology_id text,
    PRIMARY KEY (release_project_id, version_number, id),
    FOREIGN KEY (release_project_id, version_number)
        REFERENCES weave_releases (project_id, version) ON DELETE CASCADE
);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM weave_releases WHERE archive_ownership_version = 1) THEN
        RAISE EXCEPTION 'cannot downgrade verified release archives into globally colliding archive keys';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE weave_release_namespace_bindings_archive;
ALTER TABLE weave_releases DROP COLUMN archive_ownership_version;
