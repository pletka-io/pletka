-- +goose Up
-- Releases are immutable forever; archiving is the only lifecycle change
-- (design: docs/plans/2026-07-23-releases-git-design.md §1).
ALTER TABLE weave_releases ADD COLUMN archived_at timestamptz;
ALTER TABLE weave_releases ADD COLUMN archived_message text NOT NULL DEFAULT '';

-- Outbox change-set discriminator: 'draft' (existing behavior),
-- 'release', 'release_archived'. release_version carries the target
-- version for the release kinds.
ALTER TABLE weave_change_set ADD COLUMN kind text NOT NULL DEFAULT 'draft';
ALTER TABLE weave_change_set ADD COLUMN release_version text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE weave_change_set DROP COLUMN release_version;
ALTER TABLE weave_change_set DROP COLUMN kind;
ALTER TABLE weave_releases DROP COLUMN archived_message;
ALTER TABLE weave_releases DROP COLUMN archived_at;
