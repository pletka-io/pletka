-- +goose Up
-- The live table's identity is a bigserial id with a unique key on
-- (model_id, category_id, collection_id) — that composite is what allows more
-- than one collection in a category, so the archive keeps the id as its key
-- half and does not attempt to re-derive identity from the triple.
CREATE TABLE weave_collection_placements_archive (
    id             bigint NOT NULL,
    project_id     text NOT NULL,
    model_id       text NOT NULL,
    category_id    text NOT NULL DEFAULT '',
    collection_id  text NOT NULL,
    is_required    boolean NOT NULL DEFAULT false,
    min_occurs     integer NOT NULL DEFAULT 0,
    max_occurs     integer,
    is_hidden      boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    version_number text NOT NULL,
    PRIMARY KEY (id, version_number)
);

CREATE INDEX idx_wcpa_model_version ON weave_collection_placements_archive (model_id, version_number);

-- +goose Down
DROP TABLE IF EXISTS weave_collection_placements_archive;
