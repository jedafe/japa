-- 001235 down: restore the seeded out-of-taxonomy level and drop the bot
-- owner member (member_count is maintained by trg_space_member_count).
DELETE FROM space_members
WHERE space_id = (SELECT id FROM spaces WHERE slug = 'community-threads')
  AND user_id = (SELECT id FROM users WHERE username = 'community-threads');

UPDATE spaces
SET level = 'community',
    creator_user_id = NULL
WHERE slug = 'community-threads';
