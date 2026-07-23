DROP INDEX IF EXISTS idx_user_moderation_active;
ALTER TABLE user_moderation_actions DROP COLUMN IF EXISTS deleted_at;
