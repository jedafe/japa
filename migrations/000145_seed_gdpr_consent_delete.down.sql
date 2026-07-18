-- 000145_seed_gdpr_consent_delete.down.sql

BEGIN;

DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'gdpr.consents.delete');

DELETE FROM permissions WHERE key = 'gdpr.consents.delete';

COMMIT;
