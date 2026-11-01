-- 001013_verification_types.down.sql
BEGIN;

DELETE FROM role_permissions WHERE permission_id = (
    SELECT id FROM permissions WHERE key = 'verification.types.manage'
);
DELETE FROM permissions WHERE key = 'verification.types.manage';

DROP TABLE IF EXISTS verification_types;

COMMIT;
