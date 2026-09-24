-- 000721_literal_platform_name.down.sql

UPDATE template_literals
SET value = 'Tenant_Name', updated_at = now()
WHERE key = 'Tenant_Name'
  AND value = 'Platform_Name'
  AND deleted_at IS NULL;

DELETE FROM template_literals
WHERE key = 'Platform_Name';
