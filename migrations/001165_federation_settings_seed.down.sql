-- 001165_federation_settings_seed.down.sql

BEGIN;

DELETE FROM system_settings
WHERE key IN (
    'federation.outbox_page_size',
    'federation.delivery.max_attempts',
    'federation.instances.mode'
);

COMMIT;
