-- Remove control panel intelligence i18n keys (BUG-013).
DELETE FROM translations WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND key LIKE 'intel.%';
