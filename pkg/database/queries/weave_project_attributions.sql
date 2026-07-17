-- name: WeaveListProjectAttributions :many
-- Lists attribution rows for a single project, joined with the actor for
-- display name + type. Ordered by kind then position so the credits
-- panel renders authors → funders → adopters in source order.
SELECT
    a.project_id,
    a.actor_id,
    a.kind,
    a.position,
    a.note,
    a.created_at,
    act.display_name AS actor_name,
    act.type         AS actor_type
FROM weave_project_attributions a
JOIN weave_actors act ON act.id = a.actor_id
WHERE a.project_id = $1
ORDER BY
    CASE a.kind WHEN 'author' THEN 0 WHEN 'funder' THEN 1 WHEN 'adopter' THEN 2 ELSE 3 END,
    a.position ASC;

-- name: WeaveListProjectAttributionsForProjects :many
-- Same shape, batched over a set of project IDs.
SELECT
    a.project_id,
    a.actor_id,
    a.kind,
    a.position,
    a.note,
    a.created_at,
    act.display_name AS actor_name,
    act.type         AS actor_type
FROM weave_project_attributions a
JOIN weave_actors act ON act.id = a.actor_id
WHERE a.project_id = ANY($1::text[])
ORDER BY
    a.project_id,
    CASE a.kind WHEN 'author' THEN 0 WHEN 'funder' THEN 1 WHEN 'adopter' THEN 2 ELSE 3 END,
    a.position ASC;

-- name: WeaveInsertProjectAttribution :one
-- Append-style insert that picks the next position within the
-- (project, kind) bucket. Used by both the importer wave and the
-- settings POST handler.
INSERT INTO weave_project_attributions (project_id, actor_id, kind, position, note)
VALUES (
    $1, $2, $3,
    COALESCE(
        (SELECT MAX(position) + 1
         FROM weave_project_attributions
         WHERE project_id = $1 AND kind = $3),
        0
    ),
    $4
)
RETURNING project_id, actor_id, kind, position, note, created_at;

-- name: WeaveInsertProjectAttributionAt :one
-- Explicit-position variant used by the loader wave so the source
-- order of "George Bruseker, Veselina Kalkandzhieva" is preserved.
INSERT INTO weave_project_attributions (project_id, actor_id, kind, position, note)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (project_id, actor_id, kind, position) DO UPDATE SET
    note = EXCLUDED.note
RETURNING project_id, actor_id, kind, position, note, created_at;

-- name: WeaveDeleteProjectAttributionsByKind :exec
-- Wipes a (project, kind) bucket. Called by the loader before
-- re-inserting so removed names disappear on re-import. Settings UI
-- does not use this.
DELETE FROM weave_project_attributions
WHERE project_id = $1 AND kind = $2;

-- name: WeaveDeleteProjectAttribution :exec
-- Removes one specific row (the combination uniquely identifies it).
DELETE FROM weave_project_attributions
WHERE project_id = $1 AND actor_id = $2 AND kind = $3 AND position = $4;

-- name: WeaveUpdateProjectAttributionNote :exec
-- Edit the freeform "what they did" note on one row. Actor + kind +
-- position are immutable (delete + re-add to change those).
UPDATE weave_project_attributions
SET note = $5
WHERE project_id = $1 AND actor_id = $2 AND kind = $3 AND position = $4;

-- name: WeaveSetProjectAttributionPosition :exec
-- Moves one row to a new position within its (project, kind) bucket.
-- The settings reorder handler issues this once per row in the
-- requested order.
UPDATE weave_project_attributions
SET position = $5
WHERE project_id = $1 AND actor_id = $2 AND kind = $3 AND position = $4;
