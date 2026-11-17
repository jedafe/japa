-- 001186_federation_tag_discovery.down.sql
-- Reverses the M5 tag-discovery schema.

BEGIN;

DELETE FROM system_settings
 WHERE key IN ('federation.tag_subscription_limit',
               'federation.remote_search_domain_limit',
               'federation.remote_search_timeout_ms');

DROP TABLE IF EXISTS federation_tag_subscriptions;

COMMIT;
