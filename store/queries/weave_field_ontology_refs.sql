-- name: WeaveInsertFieldOntologyRef :exec
INSERT INTO weave_field_ontology_refs (
    field_id, project_id, prefix, local_name, position, ref_kind
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (field_id, position) DO NOTHING;

-- name: WeaveDeleteFieldOntologyRefs :exec
-- Bulk-delete refs for a single field. Field-slice rebuild flow:
--   tx.WeaveDeleteFieldOntologyRefs(field_id)
--   for each path element: tx.WeaveInsertFieldOntologyRef(...)
DELETE FROM weave_field_ontology_refs WHERE field_id = $1;

-- name: WeaveListFieldOntologyRefs :many
SELECT * FROM weave_field_ontology_refs
WHERE field_id = $1
ORDER BY position ASC;

-- name: WeaveFindFieldsUsingQname :many
-- "Which fields in this project reference this qname?" — drives the
-- reverse-lookup feature in the ontology explorer + unlink-preflight.
SELECT DISTINCT field_id
FROM weave_field_ontology_refs
WHERE project_id = $1 AND qname = $2
ORDER BY field_id ASC
LIMIT $3;

-- name: WeaveCountFieldsUsingQname :one
SELECT count(DISTINCT field_id)::bigint
FROM weave_field_ontology_refs
WHERE project_id = $1 AND qname = $2;

-- name: WeaveCountFieldsUsingOntologyVersion :one
-- Used by pkg/weave/projectontologyversion.Service.Delete preflight.
-- Returns the number of distinct fields in the project that reference
-- a qname defined in the given ontology version.
SELECT count(DISTINCT r.field_id)::bigint
FROM weave_field_ontology_refs r
WHERE r.project_id = $1
  AND (
    EXISTS (
      SELECT 1 FROM weave_ontology_classes c
      WHERE c.ontology_version_id = $2
        AND c.qname = r.qname
    )
    OR EXISTS (
      SELECT 1 FROM weave_ontology_properties p
      WHERE p.ontology_version_id = $2
        AND p.qname = r.qname
    )
  );

-- name: WeaveOntologyUsageByProject :one
-- "How many distinct classes/properties from this ontology version are
-- referenced by this project?" Drives the "Deployed Ontologies" card on
-- the project overview (planio #3221).
--
-- Walks every field's path_elements JSONB array directly so the count
-- reflects every class and property the project actually traverses, not
-- just root scope classes. Used to read from weave_field_ontology_refs
-- (a materialised projection of the same data) but that table was
-- never populated by the field write path, so properties always read
-- as zero — planio #3271. Reading path_elements inline keeps the count
-- correct without depending on a materialisation step.
--
-- "Referenced" combines:
--   1. every step of every field's path_elements (class or property)
--   2. entity scope classes — the ontology_scope on each model /
--      collection / field row, so a class set as a collection's main
--      scope but never traversed by a field still counts.
WITH path_steps AS (
    SELECT
        elem->>'prefix' AS prefix,
        elem->>'local_name' AS local_name,
        elem->>'type' AS step_type
    FROM weave_fields f,
         LATERAL jsonb_array_elements(coalesce(f.path_elements, '[]'::jsonb)) AS elem
    WHERE f.project_id = $1
      AND f.path_elements IS NOT NULL
), project_qnames AS (
    SELECT (prefix || ':' || local_name) AS qname, step_type AS kind
    FROM path_steps
    WHERE coalesce(prefix, '') <> '' AND coalesce(local_name, '') <> ''
    UNION
    SELECT (ontology_scope->>'prefix') || ':' || (ontology_scope->>'local_name') AS qname,
           'class'::text AS kind
    FROM weave_models
    WHERE project_id = $1
      AND ontology_scope ? 'prefix' AND ontology_scope ? 'local_name'
      AND coalesce(ontology_scope->>'prefix', '') <> ''
      AND coalesce(ontology_scope->>'local_name', '') <> ''
    UNION
    SELECT (ontology_scope->>'prefix') || ':' || (ontology_scope->>'local_name') AS qname,
           'class'::text AS kind
    FROM weave_collections
    WHERE project_id = $1
      AND ontology_scope ? 'prefix' AND ontology_scope ? 'local_name'
      AND coalesce(ontology_scope->>'prefix', '') <> ''
      AND coalesce(ontology_scope->>'local_name', '') <> ''
    UNION
    SELECT (ontology_scope->>'prefix') || ':' || (ontology_scope->>'local_name') AS qname,
           'class'::text AS kind
    FROM weave_fields
    WHERE project_id = $1
      AND ontology_scope ? 'prefix' AND ontology_scope ? 'local_name'
      AND coalesce(ontology_scope->>'prefix', '') <> ''
      AND coalesce(ontology_scope->>'local_name', '') <> ''
)
SELECT
    (
        SELECT count(DISTINCT c.qname)::bigint
        FROM weave_ontology_classes c
        WHERE c.ontology_version_id = $2
          AND c.qname IN (SELECT qname FROM project_qnames)
    ) AS classes_used,
    (
        SELECT count(DISTINCT p.qname)::bigint
        FROM weave_ontology_properties p
        WHERE p.ontology_version_id = $2
          AND p.qname IN (SELECT qname FROM project_qnames)
    ) AS properties_used;

-- name: WeaveSampleFieldsUsingOntologyVersion :many
-- Sample field IDs that block an unlink (return up to N for the 409
-- response payload). Used by the unlink-preflight to show which fields
-- reference qnames defined in the version being unlinked.
SELECT DISTINCT r.field_id, r.qname
FROM weave_field_ontology_refs r
WHERE r.project_id = $1
  AND (
    EXISTS (
      SELECT 1 FROM weave_ontology_classes c
      WHERE c.ontology_version_id = $2
        AND c.qname = r.qname
    )
    OR EXISTS (
      SELECT 1 FROM weave_ontology_properties p
      WHERE p.ontology_version_id = $2
        AND p.qname = r.qname
    )
  )
ORDER BY r.field_id ASC
LIMIT $3;
