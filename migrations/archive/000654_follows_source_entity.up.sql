-- 000654_follows_source_entity.up.sql
-- Allow the polymorphic follows table to track source follows.

BEGIN;

DO $$
DECLARE
    cname text;
BEGIN
    SELECT conname INTO cname
    FROM pg_constraint
    WHERE conrelid = 'follows'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) LIKE '%entity_type%';

    IF cname IS NOT NULL THEN
        EXECUTE format('ALTER TABLE follows DROP CONSTRAINT %I', cname);
    END IF;
END $$;

ALTER TABLE follows
    ADD CONSTRAINT follows_entity_type_check
    CHECK (entity_type IN ('user', 'space', 'tag', 'organization', 'professional_profile', 'source'));

COMMIT;
