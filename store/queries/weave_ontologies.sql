-- name: WeaveCreateOntology :one
INSERT INTO weave_ontologies (
    id, prefix, namespace, name, description, family_id,
    ontology_type, extends_ontology_id, homepage_url, source_url,
    created_by_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: WeaveGetOntologyByID :one
SELECT * FROM weave_ontologies WHERE id = $1;

-- name: WeaveGetOntologyByPrefix :one
SELECT * FROM weave_ontologies WHERE prefix = $1;

-- name: WeaveListOntologies :many
SELECT * FROM weave_ontologies
ORDER BY name ASC;

-- name: WeaveListOntologiesByFamily :many
SELECT * FROM weave_ontologies
WHERE family_id = $1
ORDER BY name ASC;

-- name: WeaveListOntologiesByType :many
SELECT * FROM weave_ontologies
WHERE ontology_type = $1
ORDER BY name ASC;

-- name: WeaveListOntologyExtensions :many
SELECT * FROM weave_ontologies
WHERE extends_ontology_id = $1
ORDER BY name ASC;

-- name: WeaveSearchOntologies :many
SELECT * FROM weave_ontologies
WHERE name ILIKE '%' || $1 || '%'
   OR prefix ILIKE '%' || $1 || '%'
   OR namespace ILIKE '%' || $1 || '%'
ORDER BY name ASC
LIMIT $2;

-- name: WeaveUpdateOntology :one
UPDATE weave_ontologies
SET prefix = $2,
    namespace = $3,
    name = $4,
    description = $5,
    family_id = $6,
    ontology_type = $7,
    extends_ontology_id = $8,
    homepage_url = $9,
    source_url = $10,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: WeaveDeleteOntology :exec
DELETE FROM weave_ontologies WHERE id = $1;

-- name: WeaveCountOntologyVersions :one
SELECT count(*)::bigint FROM weave_ontology_versions WHERE ontology_id = $1;

-- name: WeaveListNamespaceMap :many
-- Returns all (prefix, namespace) pairs ordered by namespace length DESC.
-- Used by importer + path validator to resolve URIs to qnames; longer
-- namespaces first ensures the most-specific binding wins.
SELECT prefix, namespace FROM weave_ontologies
ORDER BY length(namespace) DESC, prefix ASC;
