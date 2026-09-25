-- Add space_id to forms so they can be scoped to a space.
-- Existing forms (system seeds) remain tenant-level (space_id IS NULL).
ALTER TABLE forms ADD COLUMN space_id uuid REFERENCES spaces(id) ON DELETE CASCADE;
CREATE INDEX idx_forms_space ON forms(space_id) WHERE space_id IS NOT NULL;
