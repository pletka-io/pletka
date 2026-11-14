-- +goose Up
-- Required and Min are one concept: Required <=> Min >= 1. Repair live rows
-- where they disagree; whichever side says required wins. Release archives
-- are left as they were cut.
UPDATE weave_field_overrides
SET is_required = true,
    min_occurs  = GREATEST(COALESCE(min_occurs, 0), 1)
WHERE COALESCE(is_required, false) <> (COALESCE(min_occurs, 0) >= 1);

UPDATE weave_collection_placements
SET is_required = true,
    min_occurs  = GREATEST(min_occurs, 1)
WHERE is_required <> (min_occurs >= 1);

-- +goose Down
-- No-op: the disagreeing values are not kept.
SELECT 1;
