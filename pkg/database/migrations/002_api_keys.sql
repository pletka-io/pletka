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

CREATE INDEX weave_api_keys_actor_idx ON weave_api_keys (actor_id);

-- +goose Down
DROP TABLE weave_api_keys;
