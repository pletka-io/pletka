-- +goose Up
-- Credit is part of what a release cites.
--
-- Deviation from the (id, version_number) archive pattern, deliberately:
-- neither live table has an id. weave_project_attributions is keyed
-- (project_id, actor_id, kind) and weave_project_actors (project_id,
-- actor_id), so the archives carry those composites plus version_number. A
-- surrogate id would invent identity the live tables do not have.
CREATE TABLE weave_project_attributions_archive (
    project_id     text NOT NULL,
    actor_id       text NOT NULL,
    kind           text NOT NULL,
    position       integer NOT NULL DEFAULT 0,
    note           text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    version_number text NOT NULL,
    PRIMARY KEY (project_id, actor_id, kind, version_number)
);

CREATE TABLE weave_project_actors_archive (
    project_id     text NOT NULL,
    actor_id       text NOT NULL,
    role           text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    version_number text NOT NULL,
    PRIMARY KEY (project_id, actor_id, version_number)
);

-- +goose Down
DROP TABLE IF EXISTS weave_project_actors_archive;
DROP TABLE IF EXISTS weave_project_attributions_archive;
