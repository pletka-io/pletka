-- +goose Up
CREATE TABLE weave_api_keys (
    id           text PRIMARY KEY,
    actor_id     text NOT NULL REFERENCES weave_actors (id) ON DELETE CASCADE,
    name         text NOT NULL DEFAULT '',
    key_hash     text NOT NULL UNIQUE,
    key_prefix   text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    expires_at   timestamptz,
    revoked_at   timestamptz
);

CREATE INDEX idx_wak_actor ON weave_api_keys (actor_id);
CREATE UNIQUE INDEX idx_wak_prefix_active ON weave_api_keys (key_prefix) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE weave_api_keys;
