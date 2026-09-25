-- 000698_seed_report_reasons_setting.down.sql
-- Renumbered from 000466 (duplicate version prefix). See the .up file.
-- Remove moderation.report_reasons system setting (BUG-009 rollback).
-- De-tenanted form: system_settings.tenant_id was dropped by 000668, so the
-- original tenant filter is gone.
DELETE FROM system_settings
WHERE key = 'moderation.report_reasons';
