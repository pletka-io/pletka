-- name: OntologySnapshot :one
SELECT document,digest FROM weave_legacy_ontology_snapshots
WHERE project_id = @project_id::text AND version_number = @version::text AND digest = @digest::text;
