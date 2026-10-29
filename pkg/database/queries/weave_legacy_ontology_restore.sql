-- name: WeaveLockImportedLegacyOntologyContexts :exec
SELECT weave_lock_imported_legacy_contexts(@documents::text[]);

-- name: WeaveRestoreOntologyBody :exec
SELECT weave_restore_ontology_body(@ontology_id::text,@version_id::text,@body_text::text,@digest::text);

-- name: WeaveRetainImportedLegacyOntologySnapshot :exec
SELECT weave_retain_imported_legacy_ontology_snapshot(@document::text,@digest::text);
