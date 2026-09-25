-- Add creator_user_id to spaces (Section 05, Rule 13 circular FK).
-- The spec requires spaces.creator_user_id → users(id) ON DELETE SET NULL
-- to complete the circular FK with users.current_geo_space_id → spaces(id).
-- The column is nullable: system-seeded geo spaces have no creator.
ALTER TABLE spaces
  ADD COLUMN IF NOT EXISTS creator_user_id uuid REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_spaces_creator
  ON spaces (creator_user_id) WHERE creator_user_id IS NOT NULL;
