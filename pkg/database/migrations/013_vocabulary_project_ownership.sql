-- +goose Up
-- Vocabularies become project-owned. The global tier existed because reaching
-- Getty AAT was slow; the vocabulary service removes that constraint, and
-- shared entries let one project's resolve rewrite another project's labels.
-- See docs/plans/2026-09-26-vocabulary-project-ownership-design.md — the
-- record of which projects had the global vocabulary is kept there.

-- A concept list's source reference and its list_type both point into what is
-- about to be deleted. weave_concept_lists.vocabulary_id has no ON DELETE
-- action, so the delete below would simply fail without this. A null source is
-- an already-supported state: SearchConceptListSourceEntries branches on it
-- and falls back to the project's own vocabularies.
UPDATE weave_concept_lists SET vocabulary_id = NULL
WHERE vocabulary_id IN (SELECT id FROM weave_vocabularies WHERE project_id IS NULL);

UPDATE weave_concept_lists SET list_type = NULL
WHERE list_type IN (
    SELECT e.id FROM weave_vocabulary_entries e
    JOIN weave_vocabularies v ON v.id = e.vocabulary_id
    WHERE v.project_id IS NULL
);

-- weave_concept_list_entries.vocabulary_entry_id also has no ON DELETE
-- action, so member rows pointing at a global entry must go before the
-- entries themselves.
DELETE FROM weave_concept_list_entries
WHERE vocabulary_entry_id IN (
    SELECT e.id FROM weave_vocabulary_entries e
    JOIN weave_vocabularies v ON v.id = e.vocabulary_id
    WHERE v.project_id IS NULL
);

-- weave_concept_broader.concept_id and .broader_id both reference
-- weave_vocabulary_entries(id) ON DELETE CASCADE, so any broader/narrower
-- edge touching a deleted global entry is removed automatically by Postgres
-- when the entries are deleted below — no explicit statement needed for it.
DELETE FROM weave_vocabulary_entries
WHERE vocabulary_id IN (SELECT id FROM weave_vocabularies WHERE project_id IS NULL);

-- weave_project_vocabularies.vocabulary_id is also ON DELETE CASCADE from
-- weave_vocabularies, so this delete only pre-empts the cascade the DROP
-- TABLE below performs anyway; kept explicit for readability.
DELETE FROM weave_project_vocabularies
WHERE vocabulary_id IN (SELECT id FROM weave_vocabularies WHERE project_id IS NULL);

DELETE FROM weave_vocabularies WHERE project_id IS NULL;

DROP TABLE IF EXISTS weave_project_vocabularies;

ALTER TABLE weave_vocabularies ALTER COLUMN project_id SET NOT NULL;

-- +goose Down
-- Structural only. The dropped vocabularies, their cached entries and the
-- concept-list members that referenced them are NOT restored: they were
-- deliberately discarded as recreatable cache, and the spec records which
-- projects had them so a curator can re-add.
ALTER TABLE weave_vocabularies ALTER COLUMN project_id DROP NOT NULL;

CREATE TABLE IF NOT EXISTS weave_project_vocabularies (
    project_id text NOT NULL,
    vocabulary_id text NOT NULL,
    selected_version text,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, vocabulary_id)
);
