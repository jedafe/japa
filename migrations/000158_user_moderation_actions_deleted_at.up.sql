-- Add deleted_at column to user_moderation_actions for soft-delete support
-- (needed by ListUserWarnings and SoftDeleteUserWarning queries)
ALTER TABLE user_moderation_actions ADD COLUMN deleted_at timestamptz;
CREATE INDEX idx_user_moderation_active ON user_moderation_actions (user_id, action) WHERE deleted_at IS NULL;
