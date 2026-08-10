-- 000370 down: Restore the original (incorrect) FK referencing users(id).
-- This reverts the fix — organization_boosts.organization_id will reference
-- users(id) again. Only run this if you need to roll back.
ALTER TABLE organization_boosts DROP CONSTRAINT IF EXISTS organization_boosts_organization_id_fkey;

ALTER TABLE organization_boosts
    ADD CONSTRAINT organization_boosts_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES users(id) ON DELETE CASCADE;
