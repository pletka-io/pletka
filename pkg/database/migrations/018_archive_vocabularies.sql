-- +goose Up
-- A released concept list references vocabulary entries; without these two
-- archives those entries keep changing after the release, so the list's
-- meaning drifts from what was published.
--
-- config_encrypted is deliberately NOT archived: it holds credentials for a
-- connector, a release snapshot is read by anyone who can read the release,
-- and a stale credential is a liability rather than an asset. A reconstructed
-- read resolves the connector from the live row.
CREATE TABLE weave_vocabularies_archive (
    id             text NOT NULL,
    project_id     text NOT NULL,
    semantic_id    text,
    system_name    text,
    ui_name        jsonb,
    description    jsonb,
    status         text NOT NULL DEFAULT 'draft',
    connector_type text NOT NULL,
    base_uri       text,
    config         jsonb,
    deprecated     boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    version_number text NOT NULL,
    PRIMARY KEY (id, version_number)
);

CREATE TABLE weave_vocabulary_entries_archive (
    id                 text NOT NULL,
    vocabulary_id      text NOT NULL,
    uri                text NOT NULL,
    label              jsonb NOT NULL,
    scope_note         jsonb,
    broader_uri        text,
    broader_path       jsonb NOT NULL DEFAULT '[]'::jsonb,
    broader_path_items jsonb NOT NULL DEFAULT '[]'::jsonb,
    external_id        text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    version_number     text NOT NULL,
    PRIMARY KEY (id, version_number)
);

CREATE INDEX idx_wva_project_version ON weave_vocabularies_archive (project_id, version_number);
CREATE INDEX idx_wvea_vocabulary_version ON weave_vocabulary_entries_archive (vocabulary_id, version_number);

-- +goose Down
DROP TABLE IF EXISTS weave_vocabulary_entries_archive;
DROP TABLE IF EXISTS weave_vocabularies_archive;
