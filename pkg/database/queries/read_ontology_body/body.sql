-- name: OntologyBody :one
SELECT ontology_id, version_id, format_version, body::text AS canonical_body, digest, sealed_at
FROM weave_ontology_body_seals
WHERE ontology_id = @ontology_id::text AND version_id = @version_id::text;
