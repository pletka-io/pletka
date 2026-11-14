-- +goose Up
-- Vocabularies live in the git tree under a semantic id (<P>.VOC.<n>), so the
-- tree restore upserts them by (project_id, semantic_id).
CREATE UNIQUE INDEX IF NOT EXISTS idx_wv_project_semantic_id
    ON weave_vocabularies (project_id, semantic_id) WHERE semantic_id IS NOT NULL;

-- Per-entry config; the tree stores {"top_concept": true} for scheme top
-- concepts. NULL otherwise. The release archive mirrors the column.
ALTER TABLE weave_vocabulary_entries
    ADD COLUMN IF NOT EXISTS config jsonb;
ALTER TABLE weave_vocabulary_entries_archive
    ADD COLUMN IF NOT EXISTS config jsonb;

-- +goose Down
ALTER TABLE weave_vocabulary_entries_archive DROP COLUMN IF EXISTS config;
ALTER TABLE weave_vocabulary_entries DROP COLUMN IF EXISTS config;
DROP INDEX IF EXISTS idx_wv_project_semantic_id;
