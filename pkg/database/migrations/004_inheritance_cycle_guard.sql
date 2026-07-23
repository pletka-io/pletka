-- +goose Up
-- Reject any weave_project_inheritance write that would close an inheritance
-- cycle (including self-links). A UI-level guard exists
-- (settings.createsInheritanceCycle) but the SI <-> SUR cycle proved that
-- writers bypassing it — the Airtable import waves, git restore's raw
-- INSERTs, hand SQL — can corrupt the graph, blocking release-baseline and
-- costing a manual prod repair (2026-07-23). The database is the only layer
-- every writer goes through.
--
-- The trigger validates only NEW rows: a database that already contains a
-- cycle keeps working (reads unaffected) but cannot add more cyclic edges.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_project_inheritance_cycle() RETURNS trigger AS $$
DECLARE
    found boolean;
BEGIN
    IF NEW.project_id = NEW.parent_project_id THEN
        RAISE EXCEPTION 'inheritance cycle: project % cannot be its own parent', NEW.project_id
            USING ERRCODE = 'check_violation';
    END IF;
    -- Walk the ancestor closure of the new parent; if the child appears,
    -- this edge closes a loop.
    WITH RECURSIVE ancestors(id) AS (
        SELECT NEW.parent_project_id
        UNION
        SELECT wpi.parent_project_id
        FROM weave_project_inheritance wpi
        JOIN ancestors a ON wpi.project_id = a.id
    )
    SELECT EXISTS (SELECT 1 FROM ancestors WHERE id = NEW.project_id) INTO found;
    IF found THEN
        RAISE EXCEPTION 'inheritance cycle: % is an ancestor of % — adding % -> % would close a loop',
            NEW.project_id, NEW.parent_project_id, NEW.project_id, NEW.parent_project_id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_project_inheritance_cycle_guard
BEFORE INSERT OR UPDATE OF project_id, parent_project_id
ON weave_project_inheritance
FOR EACH ROW EXECUTE FUNCTION check_project_inheritance_cycle();

-- +goose Down
DROP TRIGGER IF EXISTS trg_project_inheritance_cycle_guard ON weave_project_inheritance;
DROP FUNCTION IF EXISTS check_project_inheritance_cycle();
