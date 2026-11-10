ALTER TABLE contents
  ADD COLUMN created_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_contents_created_by ON contents(created_by_user_id)
  WHERE deleted_at IS NULL;

ALTER TABLE forum_threads
  ADD COLUMN created_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_forum_threads_created_by ON forum_threads(created_by_user_id)
  WHERE deleted_at IS NULL;
