BEGIN;

ALTER TABLE spaces
    ADD COLUMN IF NOT EXISTS source_id uuid REFERENCES intel_sources(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_spaces_source_id ON spaces(source_id) WHERE source_id IS NOT NULL;

ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS source_id uuid REFERENCES intel_sources(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_organizations_source_id ON organizations(source_id) WHERE source_id IS NOT NULL;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS source_id uuid REFERENCES intel_sources(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_users_source_id ON users(source_id) WHERE source_id IS NOT NULL;

COMMIT;
