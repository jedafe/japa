-- 001186_federation_tag_discovery.up.sql
-- ActivityPub M5 tag discovery (§126 consume-side):
--   * federation_tag_subscriptions — member tag ingest warrants. A row
--     declares interest in a hashtag; domain '' = any domain (relay-wide).
--     Warrants widen the remote-post ingest gate beyond "actor followed"
--     and act as the per-member predicate in the timeline query.
--   * settings seed: federation.tag_subscription_limit,
--     federation.remote_search_domain_limit, federation.remote_search_timeout_ms

BEGIN;

CREATE TABLE federation_tag_subscriptions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tag          text NOT NULL,
    domain       text NOT NULL DEFAULT '',
    status       text NOT NULL DEFAULT 'active',
    follow_ap_id text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT federation_tag_subscriptions_tag_check
        CHECK (tag ~ '^[a-z0-9_]{1,64}$'),
    CONSTRAINT federation_tag_subscriptions_status_check
        CHECK (status IN ('active')),
    CONSTRAINT federation_tag_subscriptions_user_tag_domain_key
        UNIQUE (user_id, tag, domain)
);
CREATE INDEX idx_federation_tag_subs_match
    ON federation_tag_subscriptions (domain, tag) WHERE status = 'active';

INSERT INTO system_settings (key, value, is_public) VALUES
    ('federation.tag_subscription_limit', to_jsonb(25), false),
    ('federation.remote_search_domain_limit', to_jsonb(5), false),
    ('federation.remote_search_timeout_ms', to_jsonb(6000), false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
