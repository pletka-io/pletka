-- +goose Up
-- Remove redundant duplicate value-target refs and prevent recurrence.
--
-- weave_override_refs PK is (override_id, ref_type, position), so the SAME
-- target listed on one override at different positions (e.g. PDLM.9 at
-- positions 0,1,2) is legal at the storage layer but meaningless: it conveys
-- nothing extra and duplicates the Svelte {#each} key in the field editor
-- (each_key_duplicate). The Airtable import produced 4 such fields
-- (PDLF.27 / PDLF.49 / PDLF.69 / PDLF.79 -> resource_model PDLM.9 x3).
--
-- Dedupe keeps the lowest-position copy per (override_id, ref_type, target_id).
DELETE FROM weave_override_refs r
WHERE EXISTS (
    SELECT 1 FROM weave_override_refs k
    WHERE k.override_id = r.override_id
      AND k.ref_type    = r.ref_type
      AND k.target_id   = r.target_id
      AND k.position    < r.position
);

-- A field/override may target multiple DIFFERENT models of the same ref_type
-- (distinct target_id); it may never target the SAME one twice. This forbids
-- only the exact repeat, so it cannot break legitimate multi-target fields or
-- the same field reused across many models/collections (those are distinct
-- override rows).
ALTER TABLE weave_override_refs
    ADD CONSTRAINT uq_wor_override_type_target UNIQUE (override_id, ref_type, target_id);

-- +goose Down
ALTER TABLE weave_override_refs DROP CONSTRAINT IF EXISTS uq_wor_override_type_target;
