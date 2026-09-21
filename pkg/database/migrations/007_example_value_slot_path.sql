-- +goose Up
-- Example values get a slot path as their storage key.
--
-- Today a value is keyed by (example_id, override_id, occurrence_index), which
-- cannot express a second instance of a collection group or a field inside a
-- nested collection. slot_path is "<overrideID>:<occurrence>" per nesting
-- level joined by "/"; depth one equals the old key, so the backfill is
-- mechanical. override_id and occurrence_index stay populated for depth-one
-- values for now (dropped in a later migration once step B has settled).
ALTER TABLE public.weave_example_values
    ADD COLUMN IF NOT EXISTS slot_path text NOT NULL DEFAULT '';

UPDATE public.weave_example_values
   SET slot_path = override_id::text || ':' || occurrence_index::text
 WHERE slot_path = '';

ALTER TABLE public.weave_example_values
    DROP CONSTRAINT IF EXISTS weave_example_values_example_id_override_id_occurrence_inde_key;

ALTER TABLE public.weave_example_values
    ADD CONSTRAINT weave_example_values_example_id_slot_path_key UNIQUE (example_id, slot_path);

-- +goose Down
ALTER TABLE public.weave_example_values
    DROP CONSTRAINT IF EXISTS weave_example_values_example_id_slot_path_key;

ALTER TABLE public.weave_example_values
    ADD CONSTRAINT weave_example_values_example_id_override_id_occurrence_inde_key UNIQUE (example_id, override_id, occurrence_index);

ALTER TABLE public.weave_example_values
    DROP COLUMN IF EXISTS slot_path;
