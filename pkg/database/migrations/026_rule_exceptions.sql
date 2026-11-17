-- +goose Up
-- A draft that breaks a versioning rule can be grandfathered by name: one
-- project, one flag, one subject (docs-oss/architecture/versioning-and-inheritance.md).
-- The row is never deleted. An exception that ended keeps its row, so the
-- primary key refuses setting it a second time.
CREATE TABLE IF NOT EXISTS weave_rule_exceptions (
    project_id text        NOT NULL REFERENCES weave_projects (id) ON DELETE CASCADE,
    flag       text        NOT NULL CHECK (flag IN ('ontology-mixed')),
    subject    text        NOT NULL,
    note       text        NOT NULL DEFAULT '',
    set_at     timestamptz NOT NULL DEFAULT now(),
    ended_at   timestamptz,
    PRIMARY KEY (project_id, flag, subject)
);

-- +goose Down
DROP TABLE IF EXISTS weave_rule_exceptions;
