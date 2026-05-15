-- name: WeaveCreateOntologyFamily :one
INSERT INTO weave_ontology_families (
    id, slug, name, description, parent_family_id,
    homepage_url, icon, display_order
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: WeaveGetOntologyFamilyByID :one
SELECT * FROM weave_ontology_families WHERE id = $1;

-- name: WeaveGetOntologyFamilyBySlug :one
SELECT * FROM weave_ontology_families WHERE slug = $1;

-- name: WeaveListOntologyFamilies :many
SELECT * FROM weave_ontology_families
ORDER BY display_order ASC, name ASC;

-- name: WeaveListRootOntologyFamilies :many
SELECT * FROM weave_ontology_families
WHERE parent_family_id IS NULL
ORDER BY display_order ASC, name ASC;

-- name: WeaveListChildOntologyFamilies :many
SELECT * FROM weave_ontology_families
WHERE parent_family_id = $1
ORDER BY display_order ASC, name ASC;

-- name: WeaveUpdateOntologyFamily :one
UPDATE weave_ontology_families
SET slug = $2,
    name = $3,
    description = $4,
    parent_family_id = $5,
    homepage_url = $6,
    icon = $7,
    display_order = $8,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: WeaveDeleteOntologyFamily :exec
DELETE FROM weave_ontology_families WHERE id = $1;

-- name: WeaveCountOntologiesInFamily :one
SELECT count(*)::bigint FROM weave_ontologies WHERE family_id = $1;
