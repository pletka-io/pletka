-- +goose Up
-- Backfill weave_collections.default_category_id from the authoritative
-- Airtable source (Redmine #3583).
--
-- The Airtable import never populated default_category_id on collections, so
-- 726/729 rows were NULL fleet-wide. Adding a collection to a model therefore
-- dropped it into "Uncategorized" instead of its real category, and the Edit
-- Metadata form showed an empty category (it reads this column directly), even
-- though the Edit Pattern view still derived a category from the collection's
-- fields.
--
-- The true category is in the staged Airtable record:
--   weave_import_staging.raw_data -> 'fields' ->> 'Category'   e.g. "[CAT.9] Parthood"
-- We take the label's name part ("Parthood") and match it to this project's
-- own category by English UI name. Category names are unique per project
-- (verified), so the match is unambiguous. On alpha this resolved 678/678
-- collections that carry an Airtable Category with zero mismatches; the
-- remaining ~48 (SRD1 + one STA) have no Category in the source and are left
-- NULL for manual assignment.
--
-- Idempotent: only touches NULL rows. Safe no-op on a core-only database with
-- no import staging (matches nothing, 0 rows updated).
-- Note: the UPDATE target (weave_collections c) cannot be referenced inside a
-- FROM-clause JOIN's ON in Postgres, so the source tables are comma-joined and
-- every join predicate lives in WHERE.
UPDATE weave_collections c
SET default_category_id = cat.id
FROM weave_import_staging s, weave_categories cat
WHERE s.id = c.staging_id
  AND s.source_type = 'Collection'
  AND (s.raw_data->'fields') ? 'Category'
  AND cat.project_id = c.project_id
  AND cat.ui_name->>'en' = trim(split_part(s.raw_data->'fields'->>'Category', '] ', 2))
  AND c.default_category_id IS NULL;

-- +goose Down
-- Data backfill; the per-row prior NULL state was not recorded, so there is no
-- safe automatic rollback. Down is intentionally a no-op.
SELECT 1;
