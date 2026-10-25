-- 000933_verification_requests.down.sql
BEGIN;

DELETE FROM system_settings WHERE key IN ('verification.form_slug','verification.fee_credits');
DELETE FROM custom_fields WHERE id IN
  ('00000000-0000-0000-0000-00000000d022','00000000-0000-0000-0000-00000000d023',
   '00000000-0000-0000-0000-00000000d024','00000000-0000-0000-0000-00000000d014');
DELETE FROM custom_field_group_assignments WHERE group_id = '00000000-0000-0000-0000-00000000d001';
DELETE FROM forms WHERE id = '00000000-0000-0000-0000-00000000d002';
DELETE FROM custom_field_groups WHERE id = '00000000-0000-0000-0000-00000000d001';
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'verification.request');
DELETE FROM permissions WHERE key = 'verification.request';
DROP TABLE IF EXISTS verification_requests;
ALTER TABLE users DROP COLUMN IF EXISTS verification_type;
ALTER TABLE users DROP COLUMN IF EXISTS verified;

COMMIT;
