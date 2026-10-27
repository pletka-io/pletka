-- name: LockOntologyDraft :one
SELECT revision FROM weave_ontology_drafts
WHERE ontology_id = @ontology_id::text AND id = @draft_id::text FOR UPDATE;

-- name: SaveOntologyDraft :one
UPDATE weave_ontology_drafts SET document = @document::jsonb
WHERE ontology_id = @ontology_id::text AND id = @draft_id::text AND revision = @revision::bigint
RETURNING revision;
