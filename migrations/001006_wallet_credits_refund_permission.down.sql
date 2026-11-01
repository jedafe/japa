BEGIN;
DELETE FROM role_permissions WHERE permission_id = (SELECT id FROM permissions WHERE key = 'wallet.admin.credits_refund');
DELETE FROM permissions WHERE key = 'wallet.admin.credits_refund';
COMMIT;
