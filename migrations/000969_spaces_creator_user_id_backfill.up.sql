-- Backfill spaces.creator_user_id for member-created spaces.
-- The column existed since 000402 but no write path populated it until now
-- (CreateSpaceWithOwner now stamps it). Derive the creator from the earliest
-- approved 'owner' space_members row — the member who created the space.
-- Primary (platform-seeded: geo, universities) spaces have no owner member
-- and stay NULL → is_primary. Ref: §05 primary/member-created amendment.
UPDATE spaces s
SET creator_user_id = sm.user_id
FROM space_members sm
WHERE sm.space_id = s.id
  AND sm.role = 'owner'
  AND sm.status = 'approved'
  AND s.creator_user_id IS NULL
  AND sm.joined_at = (
      SELECT MIN(sm2.joined_at)
      FROM space_members sm2
      WHERE sm2.space_id = s.id
        AND sm2.role = 'owner'
        AND sm2.status = 'approved'
  );
