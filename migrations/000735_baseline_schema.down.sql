-- 000735_baseline_schema.down.sql
-- Reverts the squashed baseline by dropping the entire public schema.

BEGIN;
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO public;
COMMIT;
