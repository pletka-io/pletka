-- +goose Up
-- Credit is part of what a release cites.
--
-- Deviation from the (id, version_number) archive pattern, deliberately:
-- neither live table has an id. The live primary keys are
-- weave_project_attributions_pkey (project_id, actor_id, kind, "position")
-- and weave_project_actors_pkey (project_id, actor_id, role) — see
-- 001_baseline.sql. An archive table's primary key is the live table's
-- primary key plus version_number, so these archives carry those same
-- composites plus version_number. A surrogate id would invent identity the
-- live tables do not have; a narrower composite would let a legitimately
-- distinct live row collide under ON CONFLICT and silently vanish from the
-- snapshot.
CREATE TABLE weave_project_attributions_archive (
    project_id     text NOT NULL,
    actor_id       text NOT NULL,
    kind           text NOT NULL,
    "position"     integer NOT NULL DEFAULT 0,
    note           text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    version_number text NOT NULL,
    PRIMARY KEY (project_id, actor_id, kind, "position", version_number)
);

CREATE TABLE weave_project_actors_archive (
    project_id     text NOT NULL,
    actor_id       text NOT NULL,
    role           text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    version_number text NOT NULL,
    PRIMARY KEY (project_id, actor_id, role, version_number)
);

-- +goose Down
DROP TABLE IF EXISTS weave_project_actors_archive;
DROP TABLE IF EXISTS weave_project_attributions_archive;
