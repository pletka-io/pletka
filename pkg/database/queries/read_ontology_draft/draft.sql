-- name: OntologyDraft :one
SELECT id,ontology_id,base_version_id,revision,document FROM weave_ontology_drafts
WHERE ontology_id = @ontology_id::text AND id = @draft_id::text;
