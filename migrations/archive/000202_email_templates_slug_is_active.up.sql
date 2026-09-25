-- BUG-001: Add slug and is_active columns to email_templates for worker template lookup.
-- Backfill slug from type (they are unique per tenant) and is_active from status.

ALTER TABLE email_templates
    ADD COLUMN slug      text NOT NULL DEFAULT '',
    ADD COLUMN is_active boolean NOT NULL DEFAULT true;

-- Backfill slug from type (unique per tenant already).
UPDATE email_templates SET slug = type WHERE slug = '';

-- Backfill is_active from status.
UPDATE email_templates SET is_active = (status = 'active');

-- Add unique constraint on (tenant_id, slug) — mirrors the existing (tenant_id, type) uniqueness.
ALTER TABLE email_templates ADD CONSTRAINT email_templates_tenant_slug_uniq UNIQUE (tenant_id, slug);
