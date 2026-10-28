-- +goose Up
-- No defaults/backfill: NULL preserves the lack of evidence in older releases.
ALTER TABLE weave_projects_archive ADD COLUMN enforce_concept_lists boolean;
ALTER TABLE weave_field_overrides_archive ADD COLUMN set_value_entry_id text;
ALTER TABLE weave_concept_broader_archive ADD COLUMN created_at timestamptz;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM weave_projects_archive WHERE enforce_concept_lists IS NOT NULL)
       OR EXISTS (SELECT 1 FROM weave_field_overrides_archive WHERE set_value_entry_id IS NOT NULL)
       OR EXISTS (SELECT 1 FROM weave_concept_broader_archive WHERE created_at IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot downgrade captured release content fields';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE weave_concept_broader_archive DROP COLUMN created_at;
ALTER TABLE weave_field_overrides_archive DROP COLUMN set_value_entry_id;
ALTER TABLE weave_projects_archive DROP COLUMN enforce_concept_lists;
