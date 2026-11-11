-- +goose Up
-- A (prefix, namespace) pair is unique per scope, not across the table: a
-- project may own a binding whose pair also exists globally or in another
-- project (project.yaml decides the project's bindings, see
-- hydrateProjectNamespacesTx). Global rows carry project_id NULL or '' (legacy
-- seed data).
DROP INDEX IF EXISTS idx_wnb_prefix_namespace_unique;
CREATE UNIQUE INDEX idx_wnb_global_prefix_namespace_unique
    ON weave_namespace_bindings (prefix, namespace)
    WHERE project_id IS NULL OR project_id = '';
CREATE UNIQUE INDEX idx_wnb_project_prefix_namespace_unique
    ON weave_namespace_bindings (project_id, prefix, namespace)
    WHERE project_id <> '';

-- +goose Down
-- Fails when a project row repeats a global or another project's pair.
DROP INDEX IF EXISTS idx_wnb_project_prefix_namespace_unique;
DROP INDEX IF EXISTS idx_wnb_global_prefix_namespace_unique;
CREATE UNIQUE INDEX idx_wnb_prefix_namespace_unique
    ON weave_namespace_bindings (prefix, namespace);
