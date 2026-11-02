-- name: WeaveCreateOntologyProperty :one
INSERT INTO weave_ontology_properties (
    id, ontology_version_id, prefix, local_name, uri, label, comment,
    property_type, is_functional, is_inverse_functional, is_transitive,
    is_symmetric, is_asymmetric, is_reflexive, is_irreflexive,
    inverse_property_uri, natural_sort_key, source_module
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18
)
RETURNING *;

-- name: WeaveGetOntologyPropertyByID :one
SELECT * FROM weave_ontology_properties WHERE id = $1;

-- name: WeaveGetOntologyPropertyByURI :one
SELECT * FROM weave_ontology_properties
WHERE ontology_version_id = $1 AND uri = $2;

-- name: WeaveGetOntologyPropertyByQname :one
SELECT * FROM weave_ontology_properties
WHERE ontology_version_id = $1 AND qname = $2;

-- name: WeaveListOntologyPropertiesByVersion :many
SELECT * FROM weave_ontology_properties
WHERE ontology_version_id = $1
ORDER BY natural_sort_key ASC, local_name ASC;

-- name: WeaveListOntologyPropertiesByQnames :many
SELECT * FROM weave_ontology_properties
WHERE ontology_version_id = $1 AND qname = ANY($2::text[]);

-- name: WeaveSearchOntologyProperties :many
SELECT * FROM weave_ontology_properties
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

-- name: WeavePropertiesForDomainQname :many
-- Properties whose domain includes the given qname (in this version).
-- Used by autocomplete to suggest "what properties can start from class X".
SELECT p.*
FROM weave_ontology_properties p
JOIN weave_ontology_relations r
  ON r.source_id = p.id
 AND r.source_kind = 'property'
 AND r.rel_type = 'domain'
WHERE p.ontology_version_id = $1
  AND r.target_qname = $2
ORDER BY p.natural_sort_key ASC, p.local_name ASC;

-- name: WeavePropertiesWithoutDomain :many
-- Properties this ontology DEFINES (URI under its declared namespace) that have
-- no rdfs:domain. In RDFS such a property applies to all classes, so
-- autocomplete offers them on every class alongside the domain-matched ones.
-- Imported external properties (URI outside the namespace, e.g. dc:/foaf:/obo:)
-- are excluded so scaffolding does not flood suggestions.
SELECT p.*
FROM weave_ontology_properties p
JOIN weave_ontology_versions v ON v.id = p.ontology_version_id
JOIN weave_ontologies o ON o.id = v.ontology_id
WHERE p.ontology_version_id = $1
  AND o.namespace <> ''
  AND starts_with(p.uri, o.namespace)
  AND NOT EXISTS (
    SELECT 1
    FROM weave_ontology_relations r
    WHERE r.source_id = p.id
      AND r.source_kind = 'property'
      AND r.rel_type = 'domain'
  )
ORDER BY p.natural_sort_key ASC, p.local_name ASC;

-- name: WeavePropertiesForRangeQname :many
SELECT p.*
FROM weave_ontology_properties p
JOIN weave_ontology_relations r
  ON r.source_id = p.id
 AND r.source_kind = 'property'
 AND r.rel_type = 'range'
WHERE p.ontology_version_id = $1
  AND r.target_qname = $2
ORDER BY p.natural_sort_key ASC, p.local_name ASC;

-- name: WeaveGetInverseOntologyProperty :one
-- Resolve the inverse-property qname for a given property.
SELECT inv.*
FROM weave_ontology_relations r
JOIN weave_ontology_properties inv
  ON inv.qname = r.target_qname
 AND inv.ontology_version_id = $1
WHERE r.source_id = $2
  AND r.source_kind = 'property'
  AND r.rel_type = 'inverse_property'
LIMIT 1;

-- name: WeaveUpdateOntologyProperty :one
UPDATE weave_ontology_properties
SET prefix = $2,
    local_name = $3,
    uri = $4,
    label = $5,
    comment = $6,
    property_type = $7,
    is_functional = $8,
    is_inverse_functional = $9,
    is_transitive = $10,
    is_symmetric = $11,
    is_asymmetric = $12,
    is_reflexive = $13,
    is_irreflexive = $14,
    inverse_property_uri = $15,
    natural_sort_key = $16,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: WeaveDeleteOntologyProperty :exec
DELETE FROM weave_ontology_properties WHERE id = $1;

-- name: WeaveDeleteOntologyPropertiesByVersion :exec
DELETE FROM weave_ontology_properties WHERE ontology_version_id = $1;

-- name: WeaveCountOntologyPropertiesByVersion :one
SELECT count(*)::bigint
FROM weave_ontology_properties
WHERE ontology_version_id = $1;

-- name: WeaveListPropertiesByVersions :many
SELECT * FROM weave_ontology_properties
WHERE ontology_version_id = ANY($1::text[])
ORDER BY ontology_version_id, qname;
