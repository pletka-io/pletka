-- Preserve mode of the legacy rebuild (plan format 2). Read side first.

-- name: WeaveLegacyArchiveDigest :one
-- Plan-time fingerprint of one release's archived sections; the apply re-plans
-- and compares it, so an archive row changed in between refuses the apply.
SELECT encode(sha256(convert_to(weave_legacy_archive_sections(sqlc.arg(project_id)::text,sqlc.arg(version)::text)::text,'UTF8')),'hex')::text AS digest;

-- name: WeaveLegacyArchiveReceipt :one
SELECT weave_legacy_archive_receipt(sqlc.arg(project_id)::text,sqlc.arg(version)::text)::jsonb AS receipt;
