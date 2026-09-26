-- +goose Up
-- A project owns its vocabularies by row (migration 013): adding one IS
-- creating the row. Nothing stops the same mount being added twice, which
-- would be two rows resolving the same concepts under two different ids —
-- coherent to Postgres, useless to a curator. This index makes a second add
-- of the same mount fail instead of silently duplicating (#3599 vocabulary
-- ownership, Task 6).
--
-- A new migration rather than an edit to 013: 013 is already reviewed and
-- this repo never modifies a migration once written
-- (.claude/rules/database-patterns.md).
CREATE UNIQUE INDEX IF NOT EXISTS idx_wv_project_system_name
    ON weave_vocabularies (project_id, system_name)
    WHERE system_name IS NOT NULL AND system_name <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_wv_project_system_name;
