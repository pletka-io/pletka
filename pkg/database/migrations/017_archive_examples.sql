-- +goose Up
-- Examples are release content: a curator authors them and a reader reads
-- them, so a release that omits them shows today's examples under an old tag.
--
-- Archive tables rather than the version_number column these two tables
-- already carry: that column exists on both and is empty in every row on
-- every instance, so the in-place mechanism it anticipated was never wired.
-- weave_project_ontology_versions carries the same vestigial column beside a
-- populated _archive table, which is the pattern all sixteen existing
-- archives use and which snapshotStatements already drives. The vestigial
-- columns are left alone here; dropping them is its own change.
CREATE TABLE weave_examples_archive (
    id             text NOT NULL,
    project_id     text NOT NULL,
    entity_type    text NOT NULL,
    entity_id      text NOT NULL,
    title          jsonb,
    description    jsonb,
    status         text NOT NULL DEFAULT 'draft',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    version_number text NOT NULL,
    PRIMARY KEY (id, version_number)
);

-- example_values has a bigint id, so the archive keeps it as the identity
-- half of the key exactly as the live table numbers it.
CREATE TABLE weave_example_values_archive (
    id                    bigint NOT NULL,
    example_id            text NOT NULL,
    override_id           bigint NOT NULL,
    field_id              text NOT NULL,
    part_of_collection_id text,
    occurrence_index      integer NOT NULL DEFAULT 0,
    value_kind            text NOT NULL,
    value_payload         jsonb NOT NULL,
    text_value            text,
    number_value          numeric,
    date_value            date,
    uri_value             text,
    concept_uri           text,
    linked_example_id     text,
    slot_path             text NOT NULL DEFAULT 'root',
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    version_number        text NOT NULL,
    PRIMARY KEY (id, version_number)
);

CREATE INDEX idx_wea_project_version ON weave_examples_archive (project_id, version_number);
CREATE INDEX idx_weva_example_version ON weave_example_values_archive (example_id, version_number);

-- +goose Down
DROP TABLE IF EXISTS weave_example_values_archive;
DROP TABLE IF EXISTS weave_examples_archive;
