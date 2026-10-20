-- 000906_scholarships_slug.down.sql
-- Reverses 000906: removes the slug column and restores the registry's
-- NULL slug_column (id-addressed kind).

BEGIN;

UPDATE wiki_entity_kinds SET slug_column = NULL WHERE entity_kind = 'scholarship';
DROP INDEX IF EXISTS scholarships_slug_key;
ALTER TABLE scholarships DROP COLUMN IF EXISTS slug;

COMMIT;
