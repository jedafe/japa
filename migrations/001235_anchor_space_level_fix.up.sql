-- 001235: relevel the seeded community-threads anchor space to the valid
-- taxonomy. Migration 001091 seeded it with level='community' — a value
-- outside the space-level set (country/state/city/custom/university/
-- global) — and no creator, so it failed the member_created predicate
-- (level='custom' AND creator_user_id IS NOT NULL) and was invisible in
-- the /spaces directory while also deriving is_primary=true (wrong tab
-- matrix for a discussion-only space). Relevel to 'custom' and stamp the
-- 'community-threads' system bot as creator + owner member — the
-- CreateSpaceWithOwner shape for a bot-owned community space.
UPDATE spaces
SET level = 'custom',
    creator_user_id = (SELECT id FROM users WHERE username = 'community-threads' AND status <> 'deleted')
WHERE slug = 'community-threads';

INSERT INTO space_members (space_id, user_id, role, status)
SELECT s.id, u.id, 'owner', 'approved'
FROM spaces s
JOIN users u ON u.username = 'community-threads' AND u.status <> 'deleted'
WHERE s.slug = 'community-threads'
  AND NOT EXISTS (
    SELECT 1 FROM space_members sm
    WHERE sm.space_id = s.id AND sm.user_id = u.id
  );
