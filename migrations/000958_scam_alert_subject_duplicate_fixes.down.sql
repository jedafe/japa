BEGIN;
DELETE FROM public.role_permissions
WHERE (role_id = 'f93abe3f-06b0-4cbf-aeaf-e1be27d471a5'
   AND permission_id IN (SELECT id FROM permissions WHERE key IN ('scam_alert.review','scam_alert.pin','scam_alert.edit_any')))
   OR (role_id = '00000000-0000-0000-0000-000000000202'
   AND permission_id = (SELECT id FROM permissions WHERE key = 'scam_alert.edit_own'));
DELETE FROM public.notification_types WHERE key = 'scam_alert.reviewed';
DROP INDEX IF EXISTS idx_scam_reports_reporter;
DROP INDEX IF EXISTS idx_scam_reports_subject;
ALTER TABLE scam_reports DROP CONSTRAINT IF EXISTS scam_reports_subject_type_check;
ALTER TABLE scam_reports
    DROP COLUMN IF EXISTS subject_type,
    DROP COLUMN IF EXISTS subject_id,
    DROP COLUMN IF EXISTS subject_label,
    DROP COLUMN IF EXISTS duplicate_of_report_id;
COMMIT;
