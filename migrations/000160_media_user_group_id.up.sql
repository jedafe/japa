-- Add user_group_id to media for per-group access control.
-- NULL = universal (visible to all users in the tenant).

ALTER TABLE media ADD COLUMN user_group_id UUID REFERENCES user_groups(id) ON DELETE SET NULL;
CREATE INDEX idx_media_user_group ON media (tenant_id, user_group_id) WHERE deleted_at IS NULL;
