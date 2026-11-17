-- Reverses the tag-discovery hardening migration: restores the ASCII
-- tag CHECK and removes the live-search cache TTL setting.

BEGIN;

ALTER TABLE federation_tag_subscriptions
    DROP CONSTRAINT federation_tag_subscriptions_tag_check,
    ADD CONSTRAINT federation_tag_subscriptions_tag_check
        CHECK (tag ~ '^[a-z0-9_]{1,64}$');

DELETE FROM system_settings
 WHERE key = 'federation.remote_search_cache_ttl_ms';

COMMIT;
