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

-- name: WeaveListRelationsByVersionsAndTypes :many
SELECT r.source_id, r.source_kind, r.rel_type, r.target_qname, r.position,
       c.qname AS source_qname
FROM weave_ontology_relations r
JOIN weave_ontology_classes c
  ON c.id = r.source_id AND r.source_kind = 'class'
WHERE r.rel_type = ANY($2::text[]) AND c.ontology_version_id = ANY($1::text[])
UNION ALL
SELECT r.source_id, r.source_kind, r.rel_type, r.target_qname, r.position,
       p.qname AS source_qname
FROM weave_ontology_relations r
JOIN weave_ontology_properties p
  ON p.id = r.source_id AND r.source_kind = 'property'
WHERE r.rel_type = ANY($2::text[]) AND p.ontology_version_id = ANY($1::text[]);
