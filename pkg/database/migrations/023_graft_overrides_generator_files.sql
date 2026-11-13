-- +goose Up
-- Per-graft overrides of a model (models/<M>/grafts.yaml), keyed by address:
-- field semantic ids from the model down through the stubs.
CREATE TABLE weave_graft_overrides (
    project_id   text NOT NULL,
    model_id     text NOT NULL,
    address      text NOT NULL,
    is_hidden    boolean NOT NULL DEFAULT false,
    max_occurs   integer,
    -- max_occurs_set separates "max_occurs: null" (unbounded) from no override.
    max_occurs_set boolean NOT NULL DEFAULT false,
    display_name jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, model_id, address)
);

-- Generator sidecar files (generators/** in the tree), stored opaque.
CREATE TABLE weave_generator_files (
    project_id text NOT NULL,
    path       text NOT NULL,
    content    bytea NOT NULL,
    sha256     text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, path)
);

-- +goose Down
DROP TABLE IF EXISTS weave_generator_files;
DROP TABLE IF EXISTS weave_graft_overrides;
