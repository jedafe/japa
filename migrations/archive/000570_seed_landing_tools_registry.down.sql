-- Remove the public landing-page tool card registry.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND key = 'landing.tools';
