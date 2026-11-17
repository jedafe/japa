-- 001187_federation_tag_discovery_hardening.up.sql
-- M5 tag-discovery hardening (§126):
--   * widen the tag CHECK — subscriptions now accept unicode hashtags
--     ([\p{L}\p{N}_]{1,64} enforced in the app; PG `~` has no \p{}
--     classes so the row-level check bounds char_length only).
--   * seed the live-search per-(domain,tag) result cache TTL.

BEGIN;

ALTER TABLE federation_tag_subscriptions
    DROP CONSTRAINT federation_tag_subscriptions_tag_check,
    ADD CONSTRAINT federation_tag_subscriptions_tag_check
        CHECK (char_length(tag) BETWEEN 1 AND 64);

INSERT INTO system_settings (key, value, is_public) VALUES
    ('federation.remote_search_cache_ttl_ms', to_jsonb(60000), false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
