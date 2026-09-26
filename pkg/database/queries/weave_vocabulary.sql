-- name: WeaveCreateVocabulary :one
INSERT INTO weave_vocabularies (
    id, created_at, updated_at, semantic_id, system_name,
    ui_name, description, status, project_id,
    connector_type, base_uri, config
) VALUES (
    $1, NOW(), NOW(), $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: WeaveGetVocabulary :one
SELECT * FROM weave_vocabularies WHERE id = $1;

-- name: WeaveFindVocabularyForURI :one
SELECT * FROM weave_vocabularies
WHERE base_uri IS NOT NULL
  AND base_uri <> ''
  AND @uri::text LIKE base_uri || '%'
ORDER BY LENGTH(base_uri) DESC
LIMIT 1;

-- name: WeaveListVocabularies :many
SELECT * FROM weave_vocabularies
ORDER BY system_name ASC;

-- name: WeaveListGlobalVocabularyIDs :many
SELECT id
FROM weave_vocabularies
WHERE project_id IS NULL;

-- name: WeaveListVocabularySettingsOptions :many
-- The vocabularies a project has, for the settings screen. Every row is
-- owned by the project; "selected" is no longer a concept, because having
-- the row IS the enablement.
SELECT
    v.id,
    COALESCE(v.system_name, '') AS system_name,
    COALESCE(v.ui_name, '{}'::jsonb) AS ui_name,
    COALESCE(v.description, '{}'::jsonb) AS description,
    v.status,
    COALESCE(v.base_uri, '') AS base_uri
FROM weave_vocabularies v
WHERE v.project_id = @project_id::text
ORDER BY v.system_name ASC, v.id ASC;

-- name: WeaveListProjectScopedVocabularies :many
-- A project's vocabularies are exactly the rows carrying its project_id.
-- There is no global tier and no join table: see
-- docs/plans/2026-09-26-vocabulary-project-ownership-design.md.
SELECT v.*
FROM weave_vocabularies v
WHERE v.project_id = @project_id::text
ORDER BY v.system_name ASC, v.id ASC;

-- name: WeaveCreateVocabularyEntry :one
INSERT INTO weave_vocabulary_entries (
    id, vocabulary_id, uri, label, scope_note,
    broader_uri, broader_path, broader_path_items, external_id, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW()
) RETURNING *;

-- name: WeaveUpsertVocabularyEntry :one
INSERT INTO weave_vocabulary_entries (
    id, vocabulary_id, uri, label, scope_note,
    broader_uri, broader_path, broader_path_items, external_id, created_at, updated_at
) VALUES (
    @id::text, @vocabulary_id::text, @uri::text, @label::jsonb, @scope_note::jsonb,
    @broader_uri::text, @broader_path::jsonb, @broader_path_items::jsonb, @external_id::text, NOW(), NOW()
)
ON CONFLICT (vocabulary_id, uri) DO UPDATE SET
    label = EXCLUDED.label,
    scope_note = EXCLUDED.scope_note,
    broader_uri = EXCLUDED.broader_uri,
    broader_path = EXCLUDED.broader_path,
    broader_path_items = EXCLUDED.broader_path_items,
    external_id = EXCLUDED.external_id,
    updated_at = NOW()
RETURNING *;

-- name: WeaveGetVocabularyEntry :one
SELECT * FROM weave_vocabulary_entries WHERE id = $1;

-- name: WeaveGetVocabularyEntryByURI :one
SELECT * FROM weave_vocabulary_entries
WHERE uri = @uri::text
ORDER BY updated_at DESC
LIMIT 1;

-- name: WeaveGetVocabularyEntryByVocabularyURI :one
SELECT * FROM weave_vocabulary_entries
WHERE vocabulary_id = @vocabulary_id::text
  AND uri = @uri::text
LIMIT 1;

-- name: WeaveSearchVocabularyEntries :many
SELECT * FROM weave_vocabulary_entries
WHERE vocabulary_id = @vocabulary_id::text
  AND (
    EXISTS (SELECT 1 FROM jsonb_each_text(label) WHERE value ILIKE '%' || @query::text || '%')
    OR uri ILIKE '%' || @query::text || '%'
    OR external_id ILIKE '%' || @query::text || '%'
  )
ORDER BY uri ASC
LIMIT 50;

-- name: WeaveCreateConceptList :one
INSERT INTO weave_concept_lists (
    id, created_at, updated_at, semantic_id, system_name,
    ui_name, description, status, project_id,
    list_type, vocabulary_id
) VALUES (
    $1, NOW(), NOW(), $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: WeaveGetConceptList :one
SELECT * FROM weave_concept_lists WHERE id = $1;

-- name: WeaveGetConceptListByIDOrSemanticID :one
SELECT * FROM weave_concept_lists
WHERE id = @id::text
   OR semantic_id = @id::text
LIMIT 1;

-- name: WeaveGetProjectConceptList :one
SELECT * FROM weave_concept_lists
WHERE project_id = @project_id::text
  AND (id = @id::text OR semantic_id = @id::text);

-- name: WeaveGetConceptListNamesByIDs :many
SELECT id, semantic_id, ui_name, project_id, is_closed
FROM weave_concept_lists
WHERE id = ANY(@ids::text[])
   OR semantic_id = ANY(@ids::text[]);

-- name: WeaveListConceptLists :many
SELECT * FROM weave_concept_lists
WHERE project_id = $1
ORDER BY system_name ASC;

-- name: WeaveFindConceptListsByListType :many
SELECT * FROM weave_concept_lists
WHERE list_type = $1
ORDER BY system_name ASC;

-- name: WeaveCreateConceptListEntry :one
INSERT INTO weave_concept_list_entries (
    id, concept_list_id, vocabulary_entry_id, position,
    custom_label, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, NOW(), NOW()
) RETURNING *;

-- name: WeaveDeleteConceptListEntry :exec
DELETE FROM weave_concept_list_entries WHERE id = $1;

-- name: WeaveListConceptListEntries :many
SELECT * FROM weave_concept_list_entries
WHERE concept_list_id = $1
ORDER BY position ASC;

-- name: WeaveListConceptListEntriesWithVocabulary :many
SELECT
    cle.id AS concept_list_entry_id,
    cle.concept_list_id,
    cle.vocabulary_entry_id,
    cle.position,
    cle.custom_label,
    cle.created_at AS concept_list_entry_created_at,
    cle.updated_at AS concept_list_entry_updated_at,
    ve.id AS entry_id,
    ve.vocabulary_id,
    ve.uri,
    ve.label,
    ve.scope_note,
    ve.broader_uri,
    ve.broader_path,
    ve.broader_path_items,
    ve.external_id,
    ve.created_at AS entry_created_at,
    ve.updated_at AS entry_updated_at
FROM weave_concept_list_entries cle
JOIN weave_vocabulary_entries ve ON ve.id = cle.vocabulary_entry_id
WHERE cle.concept_list_id = @concept_list_id::text
ORDER BY cle.position ASC, ve.uri ASC;

-- name: WeaveSearchConceptListEntries :many
SELECT
    cle.id AS concept_list_entry_id,
    cle.concept_list_id,
    cle.vocabulary_entry_id,
    cle.position,
    cle.custom_label,
    cle.created_at AS concept_list_entry_created_at,
    cle.updated_at AS concept_list_entry_updated_at,
    ve.id AS entry_id,
    ve.vocabulary_id,
    ve.uri,
    ve.label,
    ve.scope_note,
    ve.broader_uri,
    ve.broader_path,
    ve.broader_path_items,
    ve.external_id,
    ve.created_at AS entry_created_at,
    ve.updated_at AS entry_updated_at
FROM weave_concept_list_entries cle
JOIN weave_vocabulary_entries ve ON ve.id = cle.vocabulary_entry_id
WHERE cle.concept_list_id = @concept_list_id::text
  AND (
    EXISTS (SELECT 1 FROM jsonb_each_text(ve.label) WHERE value ILIKE '%' || @query::text || '%')
    OR ve.uri ILIKE '%' || @query::text || '%'
    OR ve.external_id ILIKE '%' || @query::text || '%'
  )
ORDER BY cle.position ASC, ve.uri ASC
LIMIT @result_limit::integer;

-- name: WeaveUpdateConceptListEntryOrder :exec
UPDATE weave_concept_list_entries
SET position = $2, updated_at = NOW()
WHERE id = $1;

-- name: WeaveGetProjectConceptListArchive :one
SELECT * FROM weave_concept_lists_archive
WHERE project_id = @project_id::text
  AND (id = @id::text OR semantic_id = @id::text)
  AND version_number = @version_number::text;

-- name: WeaveListConceptListEntriesArchiveWithVocabulary :many
SELECT
    cle.id AS concept_list_entry_id,
    cle.concept_list_id,
    cle.vocabulary_entry_id,
    cle.position,
    cle.custom_label,
    cle.created_at AS concept_list_entry_created_at,
    cle.updated_at AS concept_list_entry_updated_at,
    ve.id AS entry_id,
    ve.vocabulary_id,
    ve.uri,
    ve.label,
    ve.scope_note,
    ve.broader_uri,
    ve.broader_path,
    ve.broader_path_items,
    ve.external_id,
    ve.created_at AS entry_created_at,
    ve.updated_at AS entry_updated_at
FROM weave_concept_list_entries_archive cle
JOIN weave_vocabulary_entries ve ON ve.id = cle.vocabulary_entry_id
WHERE cle.concept_list_id = @concept_list_id::text
  AND cle.version_number = @version_number::text
ORDER BY cle.position ASC, ve.uri ASC;

-- name: WeaveSetConceptListClosed :exec
UPDATE weave_concept_lists SET is_closed = $2, updated_at = NOW() WHERE id = $1;

-- name: WeaveListConceptListMemberURIs :many
-- Distinct member concept URIs across the given concept lists (by id or
-- semantic_id). Used to emit sealed-list value enums in generators (#3599).
SELECT DISTINCT ve.uri
FROM weave_concept_lists cl
JOIN weave_concept_list_entries cle ON cle.concept_list_id = cl.id
JOIN weave_vocabulary_entries ve ON ve.id = cle.vocabulary_entry_id
WHERE (cl.id = ANY(@ids::text[]) OR cl.semantic_id = ANY(@ids::text[]))
ORDER BY ve.uri;

-- name: WeaveConceptURIInLists :one
-- Whether a concept URI is an explicit entry of any of the given concept lists
-- (by id or semantic id). Used to validate a field's set_value against its
-- bound control list(s) (#3599).
SELECT EXISTS (
    SELECT 1
    FROM weave_concept_lists cl
    JOIN weave_concept_list_entries cle ON cle.concept_list_id = cl.id
    JOIN weave_vocabulary_entries ve ON ve.id = cle.vocabulary_entry_id AND ve.uri = @uri::text
    WHERE (cl.id = ANY(@ids::text[]) OR cl.semantic_id = ANY(@ids::text[]))
) AS present;
