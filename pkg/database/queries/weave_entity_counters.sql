-- name: WeaveAllocateEntityNumber :one
-- Atomically reserve the next sequential number for (project_id, kind).
-- Returns the value the caller should use; the counter is bumped to N+1
-- in the same statement so concurrent allocations cannot collide.
INSERT INTO weave_entity_counters (project_id, kind, next_n)
VALUES (@project_id::text, @kind::text, 2)
ON CONFLICT (project_id, kind)
DO UPDATE SET
    next_n = weave_entity_counters.next_n + 1,
    updated_at = NOW()
RETURNING (next_n - 1)::bigint;

-- name: WeaveReconcileEntityCounters :exec
-- Bump every counter so next_n > MAX(<trailing N>) for that (project, kind).
-- Idempotent. Call after bulk imports (wave importer) to close any gap
-- that direct INSERTs into entity tables would have left.
SELECT weave_reconcile_entity_counters();
