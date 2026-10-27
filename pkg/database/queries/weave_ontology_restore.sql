-- name: WeaveRestoreOntologyArtifact :exec
SELECT weave_restore_ontology_artifact(@artifact::text, @digest::text);
