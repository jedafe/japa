-- Media Quarantine Workflow
-- Adds admin review queue for false positives from virus scanning.

ALTER TABLE media ADD COLUMN quarantine_reason TEXT;
ALTER TABLE media ADD COLUMN quarantined_at TIMESTAMPTZ;
ALTER TABLE media ADD COLUMN reviewed_by UUID REFERENCES users(id);
ALTER TABLE media ADD COLUMN reviewed_at TIMESTAMPTZ;

-- Index for efficient quarantine queue queries.
CREATE INDEX idx_media_quarantined ON media (tenant_id, quarantined_at DESC)
    WHERE quarantine_reason IS NOT NULL AND deleted_at IS NULL;
