-- Add soft-delete support to contents table (AUDIT-BE-001, Rule 11 compliance)
ALTER TABLE contents ADD COLUMN deleted_at timestamptz;
CREATE INDEX idx_contents_soft_delete ON contents(tenant_id, id) WHERE deleted_at IS NULL;
