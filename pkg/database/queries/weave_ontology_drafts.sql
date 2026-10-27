-- name: WeaveCreateOntologyDraft :exec
INSERT INTO weave_ontology_drafts(id,ontology_id,base_version_id,document)
VALUES(@id,@ontology_id,@base_version_id,@document);

-- name: WeaveCurrentOntologyDraft :one
SELECT id FROM weave_ontology_drafts WHERE ontology_id=$1;

-- name: WeaveOntologyDraftPublication :one
SELECT version_id,digest,request FROM weave_ontology_draft_publications
WHERE ontology_id = @ontology_id::text AND draft_id = @draft_id::text AND revision = @revision::bigint;

-- name: WeaveRecordOntologyDraftPublication :exec
INSERT INTO weave_ontology_draft_publications(ontology_id,draft_id,revision,version_id,digest,request)
VALUES(@ontology_id,@draft_id,@revision,@version_id,@digest,@request);

-- name: WeaveOntologyDraftPublishedDigest :one
SELECT digest FROM weave_ontology_releases
WHERE ontology_id = @ontology_id::text AND version_id = @version_id::text;
