-- 001041_open_space_engagement_defaults.down.sql
-- Reverts the §05 open-by-default engagement backfill. Best-effort: rows
-- that were flipped to 'open' return to 'members'; rows an operator set to
-- 'open' deliberately after the migration would also be reverted.
BEGIN;

UPDATE space_permission_presets
SET replying_perm = 'members', commenting_perm = 'members', updated_at = now()
WHERE name = 'open_community'
  AND replying_perm = 'open' AND commenting_perm = 'open';

UPDATE space_permission_presets
SET posting_perm = 'members', replying_perm = 'members', commenting_perm = 'members',
    updated_at = now()
WHERE name = 'geo_space'
  AND posting_perm = 'open' AND replying_perm = 'open' AND commenting_perm = 'open';

ALTER TABLE spaces ALTER COLUMN commenting_perm SET DEFAULT 'members';
ALTER TABLE spaces ALTER COLUMN replying_perm SET DEFAULT 'members';

UPDATE spaces
SET posting_perm = 'members', updated_at = now()
WHERE deleted_at IS NULL
  AND visibility = 'public'
  AND posting_perm = 'open'
  AND level IN ('country', 'state', 'city', 'university', 'global');

UPDATE spaces
SET commenting_perm = 'members', updated_at = now()
WHERE deleted_at IS NULL
  AND visibility = 'public'
  AND commenting_perm = 'open';

UPDATE spaces
SET replying_perm = 'members', updated_at = now()
WHERE deleted_at IS NULL
  AND visibility = 'public'
  AND replying_perm = 'open';

COMMIT;
