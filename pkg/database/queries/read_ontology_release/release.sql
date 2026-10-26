-- name: OntologyRelease :one
SELECT artifact, digest FROM weave_ontology_releases
WHERE ontology_id = @ontology_id::text AND version_id = @version_id::text;
