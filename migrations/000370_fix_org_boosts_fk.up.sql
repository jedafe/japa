-- 000370: Fix organization_boosts.organization_id FK to reference organizations(id)
-- Ref: japaconnect-engineering/sections/25-ad-placement.md
--
-- The original migration (000024) created organization_boosts.organization_id
-- with REFERENCES users(id) — the wrong table. Per §25, it should reference
-- organizations(id). This caused CreateBoost to fail with a FK violation when
-- the provided organization_id was not also a user_id.
ALTER TABLE organization_boosts DROP CONSTRAINT IF EXISTS organization_boosts_organization_id_fkey;

ALTER TABLE organization_boosts
    ADD CONSTRAINT organization_boosts_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;
