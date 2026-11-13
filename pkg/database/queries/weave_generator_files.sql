-- name: WeaveInsertGeneratorFile :exec
INSERT INTO weave_generator_files (project_id, path, content, sha256)
VALUES ($1, $2, $3, $4);

-- name: WeaveListGeneratorFilesByProject :many
SELECT * FROM weave_generator_files
WHERE project_id = $1
ORDER BY path;

-- name: WeaveDeleteGeneratorFilesByProject :exec
DELETE FROM weave_generator_files WHERE project_id = $1;
