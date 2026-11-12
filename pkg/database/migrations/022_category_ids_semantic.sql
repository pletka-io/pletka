-- +goose Up
-- Categories are keyed by their semantic id (PREFIX.CAT.N). The category
-- service used to leave the id empty on create, so the store minted a ULID.
-- Rekey those rows and every reference to them: field overrides, collection
-- default categories, collection placements (each with its archive), category
-- adoptions and the change log. A row whose semantic id is already another
-- row's id is left alone.
CREATE TEMP TABLE category_rekey AS
SELECT DISTINCT c.id AS old_id, c.semantic_id AS new_id
FROM (
    SELECT id, semantic_id FROM weave_categories
    UNION
    SELECT id, semantic_id FROM weave_categories_archive
) c
WHERE c.semantic_id IS NOT NULL
  AND c.semantic_id <> ''
  AND c.id <> c.semantic_id
  AND NOT EXISTS (SELECT 1 FROM weave_categories o WHERE o.id = c.semantic_id);

ALTER TABLE weave_collections DROP CONSTRAINT weave_collections_default_category_fkey;

UPDATE weave_categories t SET id = r.new_id FROM category_rekey r WHERE t.id = r.old_id;
UPDATE weave_categories_archive t SET id = r.new_id FROM category_rekey r WHERE t.id = r.old_id;
UPDATE weave_field_overrides t SET category_id = r.new_id FROM category_rekey r WHERE t.category_id = r.old_id;
UPDATE weave_field_overrides_archive t SET category_id = r.new_id FROM category_rekey r WHERE t.category_id = r.old_id;
UPDATE weave_collections t SET default_category_id = r.new_id FROM category_rekey r WHERE t.default_category_id = r.old_id;
UPDATE weave_collections_archive t SET default_category_id = r.new_id FROM category_rekey r WHERE t.default_category_id = r.old_id;
UPDATE weave_collection_placements t SET category_id = r.new_id FROM category_rekey r WHERE t.category_id = r.old_id;
UPDATE weave_collection_placements_archive t SET category_id = r.new_id FROM category_rekey r WHERE t.category_id = r.old_id;
UPDATE weave_adoptions t SET source_entity_id = r.new_id FROM category_rekey r
    WHERE t.entity_type = 'category' AND t.source_entity_id = r.old_id;
UPDATE weave_adoptions_archive t SET source_entity_id = r.new_id FROM category_rekey r
    WHERE t.entity_type = 'category' AND t.source_entity_id = r.old_id;
UPDATE weave_change_log t SET entity_id = r.new_id FROM category_rekey r
    WHERE t.entity_type = 'category' AND t.entity_id = r.old_id;

ALTER TABLE weave_collections ADD CONSTRAINT weave_collections_default_category_fkey
    FOREIGN KEY (default_category_id) REFERENCES weave_categories(id) ON DELETE SET NULL;

DROP TABLE category_rekey;

-- +goose Down
-- The ULIDs are not kept, so there is nothing to restore.
SELECT 1;
