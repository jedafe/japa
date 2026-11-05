-- 001041_open_space_engagement_defaults.up.sql
-- §05 open-by-default engagement amendment: engagement axes (replying_perm,
-- commenting_perm) default to 'open' on public spaces; primary spaces
-- (country/state/city/university/global) also default posting_perm='open'.
-- Rows carrying 'members' on a public space hold the old system default,
-- not a deliberate restriction — flip them. Restrictive values a moderator
-- chose deliberately ('verified', 'moderators', 'admins') are untouched, as
-- are members/private-visibility spaces and wiki-only spaces (which carry
-- 'admins', not 'members').
BEGIN;

UPDATE spaces
SET replying_perm = 'open', updated_at = now()
WHERE deleted_at IS NULL
  AND visibility = 'public'
  AND replying_perm = 'members';

UPDATE spaces
SET commenting_perm = 'open', updated_at = now()
WHERE deleted_at IS NULL
  AND visibility = 'public'
  AND commenting_perm = 'members';

UPDATE spaces
SET posting_perm = 'open', updated_at = now()
WHERE deleted_at IS NULL
  AND visibility = 'public'
  AND posting_perm = 'members'
  AND level IN ('country', 'state', 'city', 'university', 'global');

-- Column defaults align with the new policy for any path that inserts
-- without explicit values.
ALTER TABLE spaces ALTER COLUMN replying_perm SET DEFAULT 'open';
ALTER TABLE spaces ALTER COLUMN commenting_perm SET DEFAULT 'open';

-- Seeded presets match the new defaults: geo_space is fully open (primary
-- spaces are public squares); open_community opens engagement while keeping
-- member posting (member-created space semantics).
UPDATE space_permission_presets
SET posting_perm = 'open', replying_perm = 'open', commenting_perm = 'open',
    updated_at = now()
WHERE name = 'geo_space'
  AND posting_perm = 'members' AND replying_perm = 'members' AND commenting_perm = 'members';

UPDATE space_permission_presets
SET replying_perm = 'open', commenting_perm = 'open', updated_at = now()
WHERE name = 'open_community'
  AND replying_perm = 'members' AND commenting_perm = 'members';

COMMIT;
