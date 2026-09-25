ALTER TABLE system_settings ADD COLUMN is_public boolean NOT NULL DEFAULT false;
CREATE INDEX idx_system_settings_is_public ON system_settings(tenant_id, is_public) WHERE is_public = true;
