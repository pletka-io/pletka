-- +goose Up
-- Give every project's local-terms vocabulary a usable identity.
--
-- The Airtable import created one vocabulary row per project from an Airtable
-- record rather than creating the standard local-terms row, so 42 projects on
-- each instance carry a row whose system_name is the Airtable record id
-- lowercased ("vocab_recu01yjqrnimpkpl"), whose ui_name has no "en" key, and
-- whose status is still draft. The settings screen falls back to the system
-- name when a label is missing, so a curator sees a raw record id badged
-- [Draft] where "Local terms" belongs. Reported by a user on LA, 2026-09-27.
--
-- Two different fixes, because the rows are in two different states. Measured
-- across alpha, beta and prod: 125 of the 126 rows are completely empty, and
-- exactly one (LA on alpha) holds anything.
--
-- Why dropping an empty one is safe rather than merely tolerable:
-- Service.ensureLocalVocabulary (pkg/weave/vocabulary/service.go) finds a
-- project's local vocabulary by `connector_type = 'local'`, NOT by
-- system_name, and creates a correct row when it finds none — ULID id,
-- system_name 'local_terms', ui_name "Local terms", status published,
-- base_uri 'pletka:concept/'. So deleting an unused row does not leave a
-- hole; it means the next local term a curator adds creates a well-formed
-- row. The app heals itself.

-- 1. Repair the rows that are in use, keeping their id.
--
-- An in-use row cannot be deleted and must not be: concept lists reference it
-- by id (weave_concept_lists.vocabulary_id has no ON DELETE action, so the
-- delete below would be refused), and its entries are locally minted concepts
-- a curator authored — deleting them would take the list members with them
-- via weave_vocabulary_entries' ON DELETE CASCADE.
--
-- Guarded on the project not already having a proper local_terms row:
-- migration 014's partial unique index on (project_id, system_name) would
-- reject a second one, and a project holding both is a state this migration
-- should report by leaving alone rather than fail on.
UPDATE weave_vocabularies v
SET system_name = 'local_terms',
    semantic_id = v.project_id || '.VOCAB.local',
    ui_name     = '{"en":"Local terms"}'::jsonb,
    status      = 'published',
    base_uri    = 'pletka:concept/',
    updated_at  = NOW()
WHERE v.system_name LIKE 'vocab\_rec%'
  AND v.connector_type = 'local'
  AND v.project_id IS NOT NULL
  AND (
        EXISTS (SELECT 1 FROM weave_vocabulary_entries e WHERE e.vocabulary_id = v.id)
     OR EXISTS (SELECT 1 FROM weave_concept_lists    l WHERE l.vocabulary_id = v.id)
      )
  AND NOT EXISTS (
        SELECT 1 FROM weave_vocabularies o
         WHERE o.project_id = v.project_id
           AND o.system_name = 'local_terms'
           AND o.id <> v.id
      );

-- 2. Delete the rows nothing uses.
--
-- The predicate is deliberately self-limiting. `vocab_rec%` is an Airtable id
-- shape, and if a legitimately named vocabulary ever matched it, the
-- zero-entries-and-zero-lists guard makes the match harmless: a row nothing
-- references and nothing is stored under carries no information. The
-- connector_type guard keeps this away from every service-backed vocabulary
-- regardless of name.
DELETE FROM weave_vocabularies v
WHERE v.system_name LIKE 'vocab\_rec%'
  AND v.connector_type = 'local'
  AND v.project_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM weave_vocabulary_entries e WHERE e.vocabulary_id = v.id)
  AND NOT EXISTS (SELECT 1 FROM weave_concept_lists    l WHERE l.vocabulary_id = v.id);

-- +goose Down
-- Structural only, and there is nothing structural to undo: this migration
-- changes rows, not schema.
--
-- The renamed rows keep their ids, so their references survived and nothing
-- needs restoring for them. The deleted rows cannot be recreated — they held
-- no entries and no lists referenced them, so what was lost is a row whose
-- only content was a malformed name. A project that needs a local vocabulary
-- gets a correct one from ensureLocalVocabulary on first use, which is the
-- state this migration exists to reach.
SELECT 1;
