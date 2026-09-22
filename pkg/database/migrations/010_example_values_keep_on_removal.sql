-- +goose Up
-- Example values anchor to placement (override) rows and to fields. Removing
-- a placement or a field must not silently delete worked examples: the
-- example resolver reports such values as stale and they drop out on the
-- next save from the editor (Redmine #3575, stable placements design).
ALTER TABLE weave_example_values DROP CONSTRAINT weave_example_values_override_id_fkey;
ALTER TABLE weave_example_values DROP CONSTRAINT weave_example_values_field_id_fkey;

-- +goose Down
-- Values whose placement or field is gone would block the constraints.
DELETE FROM weave_example_values v
WHERE NOT EXISTS (SELECT 1 FROM weave_field_overrides o WHERE o.id = v.override_id)
   OR NOT EXISTS (SELECT 1 FROM weave_fields f WHERE f.id = v.field_id);
ALTER TABLE weave_example_values
    ADD CONSTRAINT weave_example_values_override_id_fkey FOREIGN KEY (override_id) REFERENCES weave_field_overrides(id) ON DELETE CASCADE;
ALTER TABLE weave_example_values
    ADD CONSTRAINT weave_example_values_field_id_fkey FOREIGN KEY (field_id) REFERENCES weave_fields(id) ON DELETE CASCADE;
