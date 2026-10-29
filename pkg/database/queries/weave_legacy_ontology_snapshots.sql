-- name: WeaveCaptureLegacyOntologySnapshot :one
-- Storage derives the whole document and digest; no caller-supplied evidence.
INSERT INTO weave_legacy_ontology_snapshots(project_id,version_number)
VALUES (@project_id::text,@version::text)
RETURNING document,digest;
