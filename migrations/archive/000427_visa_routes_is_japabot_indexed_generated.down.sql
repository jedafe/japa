-- 000427_visa_routes_is_japabot_indexed_generated.down.sql
-- Reverses the generated column back to a plain boolean.

BEGIN;

ALTER TABLE visa_routes DROP COLUMN IF EXISTS is_japabot_indexed;

ALTER TABLE visa_routes
    ADD COLUMN is_japabot_indexed BOOLEAN NOT NULL DEFAULT false;

COMMIT;
