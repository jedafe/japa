-- 000407_housing_is_japabot_indexed_generated.down.sql
-- Revert is_japabot_indexed to a plain boolean default false (pre-fix state).

BEGIN;

ALTER TABLE housing_guides DROP COLUMN IF EXISTS is_japabot_indexed;

ALTER TABLE housing_guides ADD COLUMN is_japabot_indexed boolean NOT NULL DEFAULT false;

COMMIT;
