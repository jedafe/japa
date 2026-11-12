-- 001123 down — restore the status column, backfilled from is_active.
ALTER TABLE email_templates ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'active';

UPDATE email_templates SET status = CASE WHEN is_active THEN 'active' ELSE 'inactive' END;

ALTER TABLE email_templates DROP CONSTRAINT IF EXISTS email_templates_status_check;
ALTER TABLE email_templates ADD CONSTRAINT email_templates_status_check
    CHECK (status IN ('active', 'inactive'));
