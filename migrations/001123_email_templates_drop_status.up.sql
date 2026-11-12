-- 001123 — email_templates: single status source (is_active).
--
-- The table carried two sources of truth — `status` (text
-- 'active'/'inactive', ADR-055 schema) and `is_active` (boolean, added
-- 000202) — kept in sync only inside UpdateEmailTemplate. Every reader
-- (admin UI, GetEmailTemplateBySlug send path) uses is_active; `status`
-- was write-only drift risk. is_active survives; status is dropped.
--
-- Ref: .jeos/audit/email-audit.md BUG-020.

ALTER TABLE email_templates DROP CONSTRAINT IF EXISTS email_templates_status_check;
ALTER TABLE email_templates DROP COLUMN IF EXISTS status;
