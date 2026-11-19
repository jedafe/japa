BEGIN;

DELETE FROM system_settings WHERE key = 'federation.min_posts_discoverable';

COMMIT;
