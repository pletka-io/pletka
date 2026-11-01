-- name: WeaveClearOwnedExampleLinks :exec
-- Only the target's own project edits draft references on deletion. Foreign
-- consumers resolve their references through immutable releases and keep them.
UPDATE weave_example_values v
SET linked_example_id = NULL
FROM weave_examples parent, weave_examples target
WHERE target.id = sqlc.arg(target_id)::text
  AND v.linked_example_id = target.id
  AND v.example_id = parent.id
  AND parent.project_id = target.project_id;

-- name: WeaveDeleteExample :execrows
DELETE FROM weave_examples WHERE id = sqlc.arg(id)::text;
