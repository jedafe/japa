DROP INDEX IF EXISTS idx_spaces_creator;
ALTER TABLE spaces DROP COLUMN IF EXISTS creator_user_id;
