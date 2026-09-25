-- 000338_seed_attestation_guide_rbac.down.sql
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN (
    'attestation_guide.view','attestation_guide.manage','attestation_guide.update'
));
DELETE FROM permissions WHERE key IN (
    'attestation_guide.view','attestation_guide.manage','attestation_guide.update'
);
COMMIT;
