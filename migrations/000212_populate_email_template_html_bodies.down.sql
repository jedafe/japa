-- 000212_populate_email_template_html_bodies.down.sql
-- Resets all 22 email template html_body fields to empty string,
-- reverting to the state where only plain_text_body is populated.

BEGIN;

UPDATE email_templates
SET html_body = '',
    updated_at = now()
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND type IN (
    'password_reset',
    'email_verification',
    'welcome',
    'login_alert',
    'account_deletion_confirmed',
    'email_changed',
    'payment_receipt',
    'payment_failed',
    'subscription_created',
    'subscription_cancelled',
    'subscription_expiring',
    'certificate_issued',
    'course_completed',
    'ticket_confirmation',
    'ticket_status_update',
    'ticket_response',
    'daily_digest',
    'weekly_digest',
    'admin_new_user',
    'admin_content_reported',
    'admin_appeal_submitted',
    'newsletter'
  );

COMMIT;
