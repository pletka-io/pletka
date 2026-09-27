-- +goose Up
-- Removing a vocabulary from a project retires it; it does not unmake what
-- was built from it.
--
-- weave_vocabularies carried only `status`, whose recognised values are
-- draft and published (domain.Status.Valid) — there was no way to say
-- "retired" without inventing a third status value the domain rejects.
-- Deprecation is the other axis: docs-oss/architecture/domain-model.md
-- describes published + deprecated as "retired, references intact, excluded
-- from pickers", which is precisely what removal means here.
--
-- A delete cannot express it. weave_vocabulary_entries cascades from this
-- table, so deleting a vocabulary empties every concept list built on it;
-- and weave_concept_lists.vocabulary_id is a plain foreign key with no ON
-- DELETE action, so Postgres refuses the delete outright while any list
-- still points at the row. The lists must keep their entries and keep
-- resolving them — only new pins are refused.
ALTER TABLE weave_vocabularies
    ADD COLUMN IF NOT EXISTS deprecated boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE weave_vocabularies
    DROP COLUMN IF EXISTS deprecated;
