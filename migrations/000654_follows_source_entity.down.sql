-- 000654_follows_source_entity.down.sql

BEGIN;

DO $$
DECLARE
    cname text;
BEGIN
    SELECT conname INTO cname
    FROM pg_constraint
    WHERE conrelid = 'follows'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) LIKE '%source%';

    IF cname IS NOT NULL THEN
        EXECUTE format('ALTER TABLE follows DROP CONSTRAINT %I', cname);
    END IF;
END $$;

DO $$
DECLARE
    cname text;
BEGIN
    SELECT conname INTO cname
    FROM pg_constraint
    WHERE conrelid = 'follows'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) LIKE '%entity_type%';

    IF cname IS NULL THEN
        ALTER TABLE follows
            ADD CONSTRAINT follows_entity_type_check
            CHECK (entity_type IN ('user', 'space', 'tag', 'organization', 'professional_profile'));
    END IF;
END $$;

COMMIT;
