-- name: WeaveListNamespaceBindings :many
-- Returns global bindings (project_id IS NULL or empty string) plus rows
-- belonging to the given project. Legacy seed data uses empty string for
-- global; migration 022 added the column as nullable so newer rows may be NULL.
SELECT * FROM weave_namespace_bindings
WHERE project_id IS NULL OR project_id = '' OR project_id = $1
ORDER BY weight ASC, prefix ASC;

-- name: WeaveListGlobalNamespaceBindings :many
SELECT * FROM weave_namespace_bindings
WHERE project_id IS NULL OR project_id = ''
ORDER BY weight ASC, prefix ASC;

-- name: WeaveGetNamespaceBinding :one
SELECT * FROM weave_namespace_bindings
WHERE id = $1;

-- name: WeaveCreateUserNamespaceBinding :one
-- Caller generates the ULID id (matches BaseModel convention); sqlc sets source='user'.
INSERT INTO weave_namespace_bindings (
    id, project_id, prefix, namespace, weight, source, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, 'user', now(), now())
RETURNING *;

-- name: WeaveUpdateUserNamespaceBinding :one
-- Only affects rows where source='user'; system rows return 0 affected.
UPDATE weave_namespace_bindings
SET prefix     = $3,
    namespace  = $4,
    weight     = $5,
    updated_at = now()
WHERE id = $1 AND project_id = $2 AND source = 'user'
RETURNING *;

-- name: WeaveUpdateGlobalUserNamespaceBinding :one
UPDATE weave_namespace_bindings
SET prefix     = $2,
    namespace  = $3,
    weight     = $4,
    updated_at = now()
WHERE id = $1 AND source = 'user' AND (project_id IS NULL OR project_id = '')
RETURNING *;

-- name: WeaveDeleteUserNamespaceBinding :exec
-- Only deletes rows where source='user'; system rows are unaffected.
DELETE FROM weave_namespace_bindings
WHERE id = $1 AND project_id = $2 AND source = 'user';

-- name: WeaveDeleteGlobalUserNamespaceBinding :exec
DELETE FROM weave_namespace_bindings
WHERE id = $1 AND source = 'user' AND (project_id IS NULL OR project_id = '');

-- name: WeavePrefixNamespaceBindingExists :one
-- Duplicate check aligned with the (prefix, namespace) unique index.
-- Excludes the row being updated via the third parameter.
SELECT EXISTS(
  SELECT 1 FROM weave_namespace_bindings
  WHERE prefix = $1 AND namespace = $2 AND id <> $3
);

-- name: WeaveGlobalNamespaceBindingUsageByProject :many
WITH field_usage AS (
  SELECT
    f.project_id,
    COUNT(*)::bigint AS field_count
  FROM weave_fields f
  WHERE
    f.ontology_scope->>'prefix' = $1::text
    OR EXISTS (
      SELECT 1
      FROM jsonb_array_elements(COALESCE(f.path_elements, '[]'::jsonb)) pe
      WHERE pe->>'prefix' = $1::text
    )
  GROUP BY f.project_id
),
model_usage AS (
  SELECT
    m.project_id,
    COUNT(*)::bigint AS model_count
  FROM weave_models m
  WHERE m.ontology_scope->>'prefix' = $1::text
  GROUP BY m.project_id
),
collection_usage AS (
  SELECT
    c.project_id,
    COUNT(*)::bigint AS collection_count
  FROM weave_collections c
  WHERE c.ontology_scope->>'prefix' = $1::text
  GROUP BY c.project_id
)
SELECT
  p.id AS project_id,
  COALESCE(f.field_count, 0)::bigint AS field_count,
  COALESCE(m.model_count, 0)::bigint AS model_count,
  COALESCE(c.collection_count, 0)::bigint AS collection_count
FROM weave_projects p
LEFT JOIN field_usage f ON f.project_id = p.id
LEFT JOIN model_usage m ON m.project_id = p.id
LEFT JOIN collection_usage c ON c.project_id = p.id
WHERE COALESCE(f.field_count, 0) + COALESCE(m.model_count, 0) + COALESCE(c.collection_count, 0) > 0
ORDER BY
  (COALESCE(f.field_count, 0) + COALESCE(m.model_count, 0) + COALESCE(c.collection_count, 0)) DESC,
  p.id ASC;
