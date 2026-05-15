-- name: WeaveInsertOntologyRelation :exec
INSERT INTO weave_ontology_relations (
    source_id, source_kind, rel_type, target_qname, position
)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (source_id, source_kind, rel_type, target_qname) DO NOTHING;

-- name: WeaveListOntologyRelationsForSource :many
SELECT * FROM weave_ontology_relations
WHERE source_id = $1 AND source_kind = $2
ORDER BY rel_type ASC, position ASC;

-- name: WeaveListOntologyRelationsForSourceByType :many
SELECT * FROM weave_ontology_relations
WHERE source_id = $1 AND source_kind = $2 AND rel_type = $3
ORDER BY position ASC;

-- name: WeaveListOntologyRelationsForTarget :many
SELECT * FROM weave_ontology_relations
WHERE target_qname = $1 AND rel_type = $2
ORDER BY source_id ASC;

-- Recursive ancestor / descendant CTEs intentionally omitted here — sqlc's
-- parser flags ambiguity in the recursive arm when used through generated
-- code. Phase D's autocomplete subpackage runs these as inline SQL via
-- pgxpool directly.

-- name: WeaveDeleteOntologyRelationsForSource :exec
DELETE FROM weave_ontology_relations
WHERE source_id = $1 AND source_kind = $2;

-- name: WeaveDeleteOntologyRelationsForSourceByType :exec
DELETE FROM weave_ontology_relations
WHERE source_id = $1 AND source_kind = $2 AND rel_type = $3;
