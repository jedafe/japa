-- Add current_geo_space_id to users for primary geo space tracking (Section 05, ADR-005).
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS current_geo_space_id uuid REFERENCES spaces(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_users_geo_space
  ON users (current_geo_space_id) WHERE current_geo_space_id IS NOT NULL;
