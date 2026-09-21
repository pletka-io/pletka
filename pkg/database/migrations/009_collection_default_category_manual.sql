-- +goose Up
-- Backfill the 48 collections that had no category in the Airtable source, so
-- migration 008 left them NULL (Redmine #3583). Categories were supplied by
-- the curator (Vesselina) on the ticket (attachment 4160): 47 in SRD1 and one
-- in STA. Each maps 1:1 to an existing project category by English UI name
-- (verified: 48/48 resolve, zero mismatches).
--
-- Collections are keyed by their derived semantic id (project prefix + 'C.' +
-- collection_number). The UPDATE target (weave_collections c) cannot appear in
-- a FROM-clause JOIN's ON in Postgres, so the source relations are comma-joined
-- and every predicate lives in WHERE.
--
-- Idempotent (NULL-only); a no-op where the collection or category is absent.
WITH manual(sem, catname) AS (VALUES
  ('SRD1C.5','Parthood'), ('SRD1C.6','Parthood'), ('SRD1C.7','Substance'),
  ('SRD1C.8','Substance'), ('SRD1C.9','Substance'), ('SRD1C.10','Substance'),
  ('SRD1C.11','Existence'), ('SRD1C.12','Existence'), ('SRD1C.13','Events'),
  ('SRD1C.14','Events'), ('SRD1C.15','Events'), ('SRD1C.16','Existence'),
  ('SRD1C.17','Existence'), ('SRD1C.18','Existence'), ('SRD1C.19','Existence'),
  ('SRD1C.20','Substance'), ('SRD1C.21','Events'), ('SRD1C.22','Events'),
  ('SRD1C.23','Events'), ('SRD1C.24','Events'), ('SRD1C.25','Events'),
  ('SRD1C.26','Events'), ('SRD1C.27','Roles'), ('SRD1C.28','Existence'),
  ('SRD1C.29','Roles'), ('SRD1C.30','Events'), ('SRD1C.31','Actor Relations'),
  ('SRD1C.32','Actor Relations'), ('SRD1C.34','Classification'), ('SRD1C.35','Rights'),
  ('SRD1C.36','Location'), ('SRD1C.37','Location'), ('SRD1C.38','Location'),
  ('SRD1C.39','Location'), ('SRD1C.40','Description'), ('SRD1C.43','Classification'),
  ('SRD1C.44','Existence'), ('SRD1C.48','Aboutness'), ('SRD1C.49','Aboutness'),
  ('SRD1C.52','Documentation'), ('SRD1C.53','Events'), ('SRD1C.54','Substance'),
  ('SRD1C.55','Aboutness'), ('SRD1C.56','Events'), ('SRD1C.59','Events'),
  ('SRD1C.61','Events'), ('SRD1C.62','Events'), ('STAC.35','Substance')
)
UPDATE weave_collections c
SET default_category_id = cat.id
FROM manual m, weave_categories cat
WHERE (c.project_id || 'C.' || c.collection_number) = m.sem
  AND cat.project_id = c.project_id
  AND cat.ui_name->>'en' = m.catname
  AND c.default_category_id IS NULL;

-- +goose Down
-- Data backfill; prior per-row NULL state was not recorded, so there is no
-- safe automatic rollback. Down is intentionally a no-op.
SELECT 1;
