-- 000958_scam_alert_subject_duplicate_fixes.up.sql
-- §81 Scam Alert Board follow-ups:
--  * subject_* columns — the accused entity (org/user/service/other) a
--    report is about, prefilled by profile/org/service CTAs. Moderation
--    context; redacted on public surfaces like reporter identity.
--  * duplicate_of_report_id — links a report marked 'duplicate' to the
--    canonical earlier report.
--  * scam_alert.reviewed notification type — reporter is notified of the
--    review outcome.
--  * RBAC grant fixes — review/pin/edit_any to admin-group-role and
--    edit_own to business_member per §81 defaults.

BEGIN;

ALTER TABLE scam_reports
    ADD COLUMN IF NOT EXISTS subject_type text,
    ADD COLUMN IF NOT EXISTS subject_id uuid,
    ADD COLUMN IF NOT EXISTS subject_label text,
    ADD COLUMN IF NOT EXISTS duplicate_of_report_id uuid;

ALTER TABLE scam_reports DROP CONSTRAINT IF EXISTS scam_reports_subject_type_check;
ALTER TABLE scam_reports
    ADD CONSTRAINT scam_reports_subject_type_check
    CHECK (subject_type IS NULL OR subject_type IN ('user','organization','service','other'));

CREATE INDEX IF NOT EXISTS idx_scam_reports_subject ON scam_reports (subject_type, subject_id)
    WHERE subject_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_scam_reports_reporter ON scam_reports (reporter_id) WHERE reporter_id IS NOT NULL;

-- Reporter review-outcome notification type.
INSERT INTO public.notification_types (key, label, description, enabled, default_in_app, default_email, default_push, category)
VALUES ('scam_alert.reviewed', 'Scam Report Reviewed', 'A moderator reviewed your scam report', true, true, false, false, 'system')
ON CONFLICT (key) DO NOTHING;

-- §81 default grants: review/pin/edit_any → admin group role;
-- edit_own → business member (all authenticated members).
INSERT INTO public.role_permissions (role_id, permission_id, created_at)
SELECT 'f93abe3f-06b0-4cbf-aeaf-e1be27d471a5', id, now() FROM permissions
WHERE key IN ('scam_alert.review','scam_alert.pin','scam_alert.edit_any')
ON CONFLICT DO NOTHING;
INSERT INTO public.role_permissions (role_id, permission_id, created_at)
SELECT '00000000-0000-0000-0000-000000000202', id, now() FROM permissions
WHERE key = 'scam_alert.edit_own'
ON CONFLICT DO NOTHING;

COMMIT;
