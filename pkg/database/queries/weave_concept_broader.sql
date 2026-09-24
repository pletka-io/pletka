-- name: WeaveAddConceptBroader :exec
INSERT INTO weave_concept_broader (id, concept_id, broader_id, scheme_id, position)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT DO NOTHING;

-- Delete is scoped to the concept + scheme (list) so an edge id alone cannot
-- reach across tenants; RowsAffected drives a NotFound response.
-- name: WeaveRemoveConceptBroaderScoped :execrows
DELETE FROM weave_concept_broader WHERE id = $1 AND concept_id = $2 AND scheme_id = $3;

-- name: WeaveListConceptBroaderInScheme :many
SELECT id, concept_id, broader_id, scheme_id, position
FROM weave_concept_broader
WHERE concept_id = $1 AND scheme_id = $2
ORDER BY position, id;

-- name: WeaveListConceptNarrower :many
SELECT id, concept_id, broader_id, scheme_id, position
FROM weave_concept_broader
WHERE broader_id = $1
ORDER BY position, id;

-- name: WeaveConceptListEntryExists :one
SELECT EXISTS(
    SELECT 1 FROM weave_concept_list_entries
    WHERE concept_list_id = $1 AND vocabulary_entry_id = $2
) AS present;

-- name: WeaveVocabularyEntryExists :one
SELECT EXISTS(
    SELECT 1 FROM weave_vocabulary_entries WHERE id = $1
) AS present;
