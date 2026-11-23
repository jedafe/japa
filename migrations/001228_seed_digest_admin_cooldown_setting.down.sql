-- 001228_seed_digest_admin_cooldown_setting.down.sql

BEGIN;

DELETE FROM system_settings WHERE key = 'digest.admin_generate_cooldown_seconds';

COMMIT;
