-- Remove moderation.report_reasons system setting (BUG-009 rollback).
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'moderation.report_reasons';
