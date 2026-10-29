-- name: ReleaseConceptNamespace :one
SELECT a.concept_namespace, a.effective_concept_namespace::text AS effective_namespace
FROM weave_projects_archive a
JOIN weave_releases r ON r.project_id=a.id AND r.version=a.version_number
WHERE a.id=sqlc.arg(project_id)::text AND a.version_number=sqlc.arg(version)::text
  AND r.metadata_capture_version=1 AND a.effective_concept_namespace IS NOT NULL;

-- name: ReleaseModelTypes :many
SELECT a.id, a.model_type FROM weave_models_archive a
JOIN weave_releases r ON r.project_id=a.project_id AND r.version=a.version_number
WHERE a.project_id=sqlc.arg(project_id)::text AND a.version_number=sqlc.arg(version)::text
  AND r.metadata_capture_version=1 ORDER BY a.id;

-- name: ReleaseContentReceipt :one
SELECT content_capture_receipt FROM weave_releases
WHERE project_id=sqlc.arg(project_id)::text AND version=sqlc.arg(version)::text
  AND content_capture_version=1 AND content_capture_receipt IS NOT NULL;

-- name: ReleaseLegacyContentReceipt :one
SELECT content_capture_receipt FROM weave_releases
WHERE project_id=sqlc.arg(project_id)::text AND version=sqlc.arg(version)::text
  AND content_capture_version=2 AND content_capture_receipt IS NOT NULL;

-- name: ReleaseContentContract :one
SELECT content_capture_version FROM weave_releases
WHERE project_id=sqlc.arg(project_id)::text AND version=sqlc.arg(version)::text;

-- name: ReleaseLegacyOntologySnapshot :one
SELECT s.document, s.digest FROM weave_legacy_ontology_snapshots s
JOIN weave_releases r ON r.project_id=s.project_id AND r.version=s.version_number
WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
  AND r.content_capture_version=2
  AND s.digest=r.content_capture_receipt->>'ontology_snapshot_digest';
