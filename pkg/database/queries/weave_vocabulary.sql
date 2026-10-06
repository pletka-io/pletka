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
    v.deprecated,
    v.connector_type,
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

-- name: WeaveAddProjectServiceVocabulary :one
-- Enabling a service vocabulary for a project IS creating its row.
--
-- Adding a mount the project removed earlier REVIVES that row rather than
-- inserting a second one. Removal deprecates rather than deletes, so the old
-- row still holds (project_id, system_name) in the partial unique index from
-- migration 014 — without the upsert, removing a vocabulary would be a
-- one-way door, and the add would fail with "already added to this project"
-- while the screen showed it as removed. Reviving also brings back the
-- entries cached under that row, which is what a curator re-adding a source
-- they had pinned terms from would expect.
--
-- The DO UPDATE is guarded on the existing row being deprecated, so adding a
-- mount that is genuinely still there updates nothing and returns no row —
-- which the store maps to the 409 conflict. The conflict target repeats the
-- index's predicate so Postgres can infer the partial index.
INSERT INTO weave_vocabularies (id, project_id, system_name, ui_name, base_uri, connector_type, config, status, created_at, updated_at)
VALUES (@id::text, @project_id::text, @system_name::text, @ui_name::jsonb, NULLIF(@base_uri::text, ''), 'vocabservice', @config::jsonb, 'published', NOW(), NOW())
ON CONFLICT (project_id, system_name) WHERE system_name IS NOT NULL AND system_name <> ''
DO UPDATE SET
    deprecated = false,
    status = 'published',
    ui_name = EXCLUDED.ui_name,
    base_uri = EXCLUDED.base_uri,
    config = EXCLUDED.config,
    updated_at = NOW()
WHERE weave_vocabularies.deprecated
RETURNING id;

-- name: WeaveDeprecateProjectVocabulary :one
-- Removing a vocabulary deprecates its row rather than deleting it.
--
-- A delete cannot express what removal means here. weave_vocabulary_entries
-- cascades from weave_vocabularies, so deleting takes every pinned entry with
-- it and empties the concept lists built on them; and weave_concept_lists has
-- a plain foreign key with no ON DELETE action, so Postgres refuses the delete
-- outright while any list still points at the row. Removal has to leave the
-- row in place: the lists keep their entries and keep resolving them, and the
-- vocabulary simply stops being offered for anything new.
UPDATE weave_vocabularies
SET deprecated = true, updated_at = NOW()
WHERE id = @id::text AND project_id = @project_id::text
  -- The project's local-terms row is not removable. It is the fallback for
  -- terms no thesaurus has, every project needs one, and a concept list with
  -- no source vocabulary resolves against it — retiring it would leave a
  -- curator unable to add a term anywhere. Enforced here rather than only in
  -- the UI, so the endpoint cannot be asked to do it directly.
  AND connector_type <> 'local'
RETURNING id;

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

-- ===========================================================================
-- Queries moved out of pkg/weave/vocabulary/service.go, where they were Go
-- string literals. Moved verbatim: a behaviour change hidden inside a
-- mechanical move is the hardest kind to find later, so any improvement to
-- these is a separate commit with its own test.
-- ===========================================================================

-- name: WeaveListAdminVocabularies :many
-- Administrative inventory of every vocabulary with its project and entry count.  (was ListAdminVocabularies)
SELECT
    v.id,
    COALESCE(v.semantic_id, '') AS semantic_id,
    COALESCE(v.system_name, '') AS system_name,
    COALESCE(v.ui_name, '{}'::jsonb) AS ui_name,
    COALESCE(v.description, '{}'::jsonb) AS description,
    v.status,
    v.project_id,
    v.connector_type,
    COALESCE(v.base_uri, '') AS base_uri,
    v.created_at,
    v.updated_at,
    COUNT(DISTINCT ve.id) AS entry_count,
    COUNT(DISTINCT v.project_id) AS project_count
FROM weave_vocabularies v
LEFT JOIN weave_vocabulary_entries ve ON ve.vocabulary_id = v.id
GROUP BY v.id
ORDER BY v.system_name ASC, v.id ASC;

-- name: WeaveConceptListDecoration :many
-- The vocabulary and bound-field decoration beside each of a project's concept lists.  (was decorateConceptLists)
SELECT
    cl.id,
    COALESCE(v.id, '') AS vocabulary_id,
    COALESCE(v.semantic_id, '') AS vocabulary_semantic_id,
    COALESCE(v.system_name, '') AS vocabulary_system_name,
    COALESCE(v.ui_name, '{}'::jsonb) AS vocabulary_ui_name,
    COALESCE(v.base_uri, '') AS vocabulary_base_uri,
    COALESCE(lte.id, '') AS list_type_id,
    COALESCE(lte.vocabulary_id, '') AS list_type_vocabulary_id,
    COALESCE(lte.uri, '') AS list_type_uri,
    COALESCE(lte.label, '{}'::jsonb) AS list_type_label,
    COALESCE(lte.scope_note, '{}'::jsonb) AS list_type_scope_note,
    COALESCE(lte.broader_uri, '') AS list_type_broader_uri,
    COALESCE(lte.external_id, '') AS list_type_external_id,
    COUNT(DISTINCT cle.id) AS entry_count,
    COUNT(DISTINCT fo.field_id) AS bound_field_count
FROM weave_concept_lists cl
LEFT JOIN weave_vocabularies v ON v.id = cl.vocabulary_id
LEFT JOIN weave_vocabulary_entries lte ON lte.id = cl.list_type
LEFT JOIN weave_concept_list_entries cle ON cle.concept_list_id = cl.id
LEFT JOIN weave_override_refs r
    ON r.ref_type = 'concept_list'
    AND (r.target_id = cl.id OR r.semantic_id = cl.semantic_id)
LEFT JOIN weave_field_overrides fo
    ON fo.id = r.override_id
    AND fo.project_id = cl.project_id
WHERE cl.project_id = $1
GROUP BY cl.id, v.id, v.semantic_id, v.system_name, v.ui_name, v.base_uri, lte.id, lte.vocabulary_id, lte.uri, lte.label, lte.scope_note, lte.broader_uri, lte.external_id;

-- name: WeaveUpdateConceptList :exec
-- Update a concept list's editable fields.  (was UpdateConceptList)
UPDATE weave_concept_lists
SET system_name = $3,
    ui_name = $4::jsonb,
    description = $5::jsonb,
    status = $6,
    vocabulary_id = $7,
    list_type = $8,
    updated_at = NOW()
WHERE project_id = $1
  AND (id = $2 OR semantic_id = $2);

-- name: WeaveConceptListOverrideRefCount :one
-- How many distinct fields reference the list through an override; the delete guard.  (was DeleteConceptList)
SELECT COUNT(DISTINCT fo.field_id)
FROM weave_override_refs r
JOIN weave_field_overrides fo ON fo.id = r.override_id
WHERE fo.project_id = $1
  AND r.ref_type = 'concept_list'
  AND (r.target_id = $2 OR r.semantic_id = $3);

-- name: WeaveDeleteConceptList :exec
-- Delete a concept list by id or semantic id.  (was DeleteConceptList)
DELETE FROM weave_concept_lists
WHERE project_id = $1
  AND (id = $2 OR semantic_id = $2);

-- name: WeaveFindConceptListEntryByVocabEntry :one
-- Find an existing entry so adding the same term twice is idempotent.  (was AddConceptListEntry)
SELECT id
FROM weave_concept_list_entries
WHERE concept_list_id = $1 AND vocabulary_entry_id = $2
LIMIT 1;

-- name: WeaveNextConceptListEntryPosition :one
-- The next position in a list; MAX(position)+1.  (was AddConceptListEntry)
SELECT COALESCE(MAX(position), 0) + 1
FROM weave_concept_list_entries
WHERE concept_list_id = $1;

-- name: WeaveNextLocalTermPosition :one
-- The next position when appending a locally created term.  (was CreateLocalTerm)
SELECT COALESCE(MAX(position), 0) + 1 FROM weave_concept_list_entries WHERE concept_list_id = $1;

-- name: WeaveFindLocalVocabulary :one
-- The project's local vocabulary, if it already has one.  (was ensureLocalVocabulary)
SELECT id FROM weave_vocabularies
WHERE project_id = $1 AND connector_type = 'local'
ORDER BY created_at LIMIT 1;

-- name: WeaveDeleteConceptListEntryInList :exec
-- Remove one entry from a list.  (was RemoveConceptListEntry)
DELETE FROM weave_concept_list_entries
WHERE id = $1 AND concept_list_id = $2;
-- Scoped to the list as well as the entry id. The existing WeaveDeleteConceptListEntry is not:
-- a wrong id there can touch an entry in another list. That looser form has
-- one legacy caller and is left alone here rather than changed underneath it.
-- name: WeaveUpdateConceptListEntryLabel :exec
-- Set an entry's custom label.  (was UpdateConceptListEntry)
UPDATE weave_concept_list_entries
SET custom_label = $1::jsonb,
    updated_at = NOW()
WHERE id = $2
  AND concept_list_id = $3;

-- name: WeaveSetConceptListEntryPositionInList :exec
-- Set one entry's position; the reorder loop calls it per id.  (was ReorderConceptListEntries)
UPDATE weave_concept_list_entries
SET position = $1,
    updated_at = NOW()
WHERE id = $2
  AND concept_list_id = $3;
-- Scoped to the list as well as the entry id. The existing WeaveSetConceptListEntryPosition is not:
-- a wrong id there can touch an entry in another list. That looser form has
-- one legacy caller and is left alone here rather than changed underneath it.
-- name: WeaveMaxConceptListNumber :one
-- Highest existing CL number for a project, scanned from the id.  (was nextConceptListID)
SELECT COALESCE(MAX((regexp_match(id, '^' || $1 || '\.CL\.([0-9]+)$'))[1]::bigint), 0)
FROM weave_concept_lists
WHERE project_id = $1
  AND id ~ ('^' || $1 || '\.CL\.[0-9]+$');

-- name: WeaveReserveConceptListCounter :exec
-- Advance and return the project's concept-list counter.  (was nextConceptListID)
INSERT INTO weave_entity_counters (project_id, kind, next_n)
VALUES ($1, 'concept_list', $2)
ON CONFLICT (project_id, kind)
DO UPDATE SET
    next_n = GREATEST(weave_entity_counters.next_n, EXCLUDED.next_n),
    updated_at = NOW();
