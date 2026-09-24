-- +goose Up
-- Concept Model (SKOS controlled vocabularies) phase 1 — schema only.
-- The default per-project local vocabulary is provisioned by an idempotent
-- app-boot ensureLocalVocabulary (ULID PKs are minted in Go), not here.

-- A concept list (skos:ConceptScheme) can be sealed: membership is complete.
ALTER TABLE weave_concept_lists
    ADD COLUMN IF NOT EXISTS is_closed boolean NOT NULL DEFAULT false;

-- Editable broader/narrower edges between concepts (skos:broader).
-- scheme_id NULL = a global edge not scoped to one scheme (cross-scheme).
CREATE TABLE weave_concept_broader (
    id            text NOT NULL PRIMARY KEY,
    concept_id    text NOT NULL REFERENCES weave_vocabulary_entries(id) ON DELETE CASCADE,
    broader_id    text NOT NULL REFERENCES weave_vocabulary_entries(id) ON DELETE CASCADE,
    scheme_id     text REFERENCES weave_concept_lists(id) ON DELETE CASCADE,
    position      integer NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT no_self_broader CHECK (concept_id <> broader_id)
);
-- Uniqueness: nullable scheme_id means we cannot use a plain table constraint
-- (NULLs are distinct), so use two partial unique indexes.
CREATE UNIQUE INDEX uq_concept_broader_scheme
    ON weave_concept_broader (concept_id, broader_id, scheme_id)
    WHERE scheme_id IS NOT NULL;
CREATE UNIQUE INDEX uq_concept_broader_global
    ON weave_concept_broader (concept_id, broader_id)
    WHERE scheme_id IS NULL;
CREATE INDEX idx_concept_broader_concept ON weave_concept_broader (concept_id);
CREATE INDEX idx_concept_broader_scheme ON weave_concept_broader (scheme_id);

-- Release archival of the hierarchy edges (mirrors weave_concept_lists_archive).
CREATE TABLE weave_concept_broader_archive (
    id             text NOT NULL,
    concept_id     text NOT NULL,
    broader_id     text NOT NULL,
    scheme_id      text NOT NULL,
    position       integer NOT NULL DEFAULT 0,
    version_number text NOT NULL,
    PRIMARY KEY (id, version_number)
);

-- +goose Down
DROP TABLE IF EXISTS weave_concept_broader_archive;
DROP TABLE IF EXISTS weave_concept_broader;
ALTER TABLE weave_concept_lists DROP COLUMN IF EXISTS is_closed;
