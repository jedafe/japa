ALTER TABLE email_templates DROP CONSTRAINT IF EXISTS email_templates_tenant_slug_uniq;
ALTER TABLE email_templates DROP COLUMN IF EXISTS slug;
ALTER TABLE email_templates DROP COLUMN IF EXISTS is_active;
