-- 000209_user_profiles_media_ids.up.sql
-- Add avatar_media_id and header_media_id to user_profiles for media-system
-- integration per Section 50 (PATCH /me/profile spec).
-- Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md (BUG-004)
-- The existing avatar_url/cover_url columns are kept for backward compatibility;
-- when a media_id is provided, the service resolves it to a URL and stores both.

ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS avatar_media_id uuid REFERENCES media(id) ON DELETE SET NULL;
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS header_media_id uuid REFERENCES media(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_user_profiles_avatar_media ON user_profiles(avatar_media_id) WHERE avatar_media_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_user_profiles_header_media ON user_profiles(header_media_id) WHERE header_media_id IS NOT NULL;
