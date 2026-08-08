-- 000347_bookmark_folders_position.up.sql
-- BL-001: Adds the `position` column to bookmark_folders per §08 spec.
-- Allows manual folder ordering. Zero-downtime: additive, nullable with
-- default 0, no backfill needed (existing rows get position = 0).

BEGIN;

ALTER TABLE bookmark_folders
    ADD COLUMN IF NOT EXISTS position int NOT NULL DEFAULT 0;

COMMIT;
