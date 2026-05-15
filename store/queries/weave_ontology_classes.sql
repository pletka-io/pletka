-- name: WeaveCreateOntologyClass :one
INSERT INTO weave_ontology_classes (
    id, ontology_version_id, prefix, local_name, uri, label, comment, natural_sort_key
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: WeaveGetOntologyClassByID :one
SELECT * FROM weave_ontology_classes WHERE id = $1;

-- name: WeaveGetOntologyClassByURI :one
SELECT * FROM weave_ontology_classes
WHERE ontology_version_id = $1 AND uri = $2;

-- name: WeaveGetOntologyClassByQname :one
SELECT * FROM weave_ontology_classes
WHERE ontology_version_id = $1 AND qname = $2;

-- name: WeaveListOntologyClassesByVersion :many
SELECT * FROM weave_ontology_classes
WHERE ontology_version_id = $1
ORDER BY natural_sort_key ASC, local_name ASC;

-- name: WeaveListSubclassesByQname :many
-- Return classes that are direct subclasses of target_qname within
-- a specific version. Drives autocomplete range expansion: P1's range
-- is crm:E41_Appellation, but the dropdown should also offer
-- crm:E33_E41_Linguistic_Appellation since it subclasses E41
-- (planio #3239 / #3248). Caller does breadth-first descent in Go for
-- transitive coverage.
SELECT c.* FROM weave_ontology_classes c
JOIN weave_ontology_relations r
  ON r.source_id = c.id
 AND r.source_kind = 'class'
 AND r.rel_type = 'subclass_of'
WHERE c.ontology_version_id = $1
  AND r.target_qname = $2
ORDER BY c.natural_sort_key ASC, c.local_name ASC;

-- name: WeaveListOntologyClassesByQnames :many
-- Batch-load classes by qname within a version. Used by autocomplete to
-- avoid N+1 lookups when materializing a path's class metadata.
SELECT * FROM weave_ontology_classes
WHERE ontology_version_id = $1 AND qname = ANY($2::text[]);

-- name: WeaveSearchOntologyClasses :many
-- Substring search over local_name, qname, and label JSONB values.
-- Limited result set for autocomplete-style use.
SELECT * FROM weave_ontology_classes
WHERE ontology_version_id = $1
  AND (
        local_name ILIKE '%' || $2 || '%'
     OR qname ILIKE '%' || $2 || '%'
     OR label::text ILIKE '%' || $2 || '%'
      )
ORDER BY
    CASE WHEN local_name ILIKE $2 || '%' THEN 0 ELSE 1 END,
    natural_sort_key ASC, local_name ASC
LIMIT $3;

-- name: WeaveUpdateOntologyClass :one
UPDATE weave_ontology_classes
SET prefix = $2,
    local_name = $3,
    uri = $4,
    label = $5,
    comment = $6,
    natural_sort_key = $7,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: WeaveDeleteOntologyClass :exec
DELETE FROM weave_ontology_classes WHERE id = $1;

-- name: WeaveDeleteOntologyClassesByVersion :exec
DELETE FROM weave_ontology_classes WHERE ontology_version_id = $1;

-- name: WeaveCountOntologyClassesByVersion :one
SELECT count(*)::bigint
FROM weave_ontology_classes
WHERE ontology_version_id = $1;
