-- name: WeaveAddConceptBroader :exec
INSERT INTO weave_concept_broader (id, concept_id, broader_id, scheme_id, position)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT DO NOTHING;

-- name: WeaveRemoveConceptBroader :exec
DELETE FROM weave_concept_broader WHERE id = $1;

-- name: WeaveListConceptBroader :many
SELECT id, concept_id, broader_id, scheme_id, position
FROM weave_concept_broader
WHERE concept_id = $1
ORDER BY position, id;

-- name: WeaveListConceptNarrower :many
SELECT id, concept_id, broader_id, scheme_id, position
FROM weave_concept_broader
WHERE broader_id = $1
ORDER BY position, id;
