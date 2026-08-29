BEGIN;

ALTER TABLE notification_types DROP COLUMN IF EXISTS category;

COMMIT;
