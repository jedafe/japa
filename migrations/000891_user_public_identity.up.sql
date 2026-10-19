-- 000891_user_public_identity.up.sql — super-admin public identity fields.
-- Ref: .jeos/sections/03-rbac.md (Amendment — Super-Admin Public Identity).
--
-- public_username is the alias handle rendered on public surfaces in place of
-- users.username (COALESCE at read time). public_group_id is the group whose
-- name is shown publicly as the user's role; it must reference a group the
-- user belongs to (enforced at the app layer) and clears itself if the group
-- is deleted.
--
-- All additive — zero downtime.
BEGIN;

ALTER TABLE users ADD COLUMN public_username text;
ALTER TABLE users ADD COLUMN public_group_id uuid REFERENCES user_groups(id) ON DELETE SET NULL;

-- Aliases must be unique among themselves, case-insensitively. Collision
-- against canonical usernames is checked at the app layer (cross-column
-- uniqueness cannot be expressed as a single index).
CREATE UNIQUE INDEX idx_users_public_username ON users (lower(public_username)) WHERE public_username IS NOT NULL;

COMMIT;
