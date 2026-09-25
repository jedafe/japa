-- 000342_seed_tax_calculator_rbac.down.sql
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN (
    'tax_calculator.view','tax_calculator.manage','tax_content.review'
));
DELETE FROM permissions WHERE key IN (
    'tax_calculator.view','tax_calculator.manage','tax_content.review'
);
COMMIT;
