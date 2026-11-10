ALTER TABLE comments
  ADD COLUMN created_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_comments_created_by ON comments(created_by_user_id)
  WHERE deleted_at IS NULL;

ALTER TABLE forum_replies
  ADD COLUMN created_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_forum_replies_created_by ON forum_replies(created_by_user_id)
  WHERE deleted_at IS NULL;
