-- name: WeaveSealOntologyVersionBody :exec
-- The database builds the body and digest itself under the same lock used by
-- all body writers. Callers cannot supply a fabricated artifact or seal marker.
INSERT INTO weave_ontology_body_seals (ontology_id, version_id)
VALUES (@ontology_id::text, @version_id::text);

-- name: WeaveAuditOrphanOntologyRelations :many
SELECT source_kind, source_id, rel_type, target_qname
FROM weave_ontology_relations WHERE ontology_version_id IS NULL
ORDER BY source_kind, source_id, rel_type, target_qname;

-- name: WeaveClearActiveOntologyVersions :exec
UPDATE weave_ontology_versions SET is_active = false WHERE ontology_id = $1 AND is_active;

-- name: WeaveActivateOntologyVersion :execrows
UPDATE weave_ontology_versions SET is_active = true WHERE ontology_id = $1 AND id = $2;
