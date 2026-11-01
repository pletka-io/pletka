-- name: BrowserFieldUsage :one
SELECT count(DISTINCT entity_id) FILTER (WHERE entity_type = 'model')::integer AS models_using,
       count(DISTINCT entity_id) FILTER (WHERE entity_type = 'collection')::integer AS collections_using,
       count(*) FILTER (WHERE entity_type IN ('model', 'collection'))::integer AS override_count
FROM weave_field_overrides_archive
WHERE project_id = sqlc.arg(project_id) AND version_number = sqlc.arg(version) AND field_id = sqlc.arg(field_id);
