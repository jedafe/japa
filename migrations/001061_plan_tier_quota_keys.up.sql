-- 001061_plan_tier_quota_keys.up.sql
-- Completes the 001060 coverage: seeds {regular,pro,business} variants for
-- the per-user quota families so plan tiers also get differentiated,
-- admin-tunable limits instead of sharing the flat key.

BEGIN;

INSERT INTO system_settings (key, value, updated_at) VALUES
    ('api.keys.max_per_user.regular',                 to_jsonb(5),    now()),
    ('api.keys.max_per_user.pro',                     to_jsonb(10),   now()),
    ('api.keys.max_per_user.business',                to_jsonb(25),   now()),
    ('affiliate.max_links_per_user.regular',          to_jsonb(10),   now()),
    ('affiliate.max_links_per_user.pro',              to_jsonb(25),   now()),
    ('affiliate.max_links_per_user.business',         to_jsonb(50),   now()),
    ('cv_formatter.quota_standard.regular',           to_jsonb(3),    now()),
    ('cv_formatter.quota_standard.pro',               to_jsonb(10),   now()),
    ('cv_formatter.quota_standard.business',          to_jsonb(25),   now()),
    ('agent.rate_limit_per_day.regular',              to_jsonb(50),   now()),
    ('agent.rate_limit_per_day.pro',                  to_jsonb(150),  now()),
    ('agent.rate_limit_per_day.business',             to_jsonb(500),  now()),
    ('ai.byok.daily_query_cap.regular',               to_jsonb(200),  now()),
    ('ai.byok.daily_query_cap.pro',                   to_jsonb(500),  now()),
    ('ai.byok.daily_query_cap.business',              to_jsonb(2000), now()),
    ('japa_stage.max_journeys.regular',               to_jsonb(10),   now()),
    ('japa_stage.max_journeys.pro',                   to_jsonb(25),   now()),
    ('japa_stage.max_journeys.business',              to_jsonb(50),   now()),
    ('referral.max_rewards_per_period.regular',       to_jsonb(10),   now()),
    ('referral.max_rewards_per_period.pro',           to_jsonb(25),   now()),
    ('referral.max_rewards_per_period.business',      to_jsonb(50),   now()),
    ('mentorship.max_capacity.regular',               to_jsonb(10),   now()),
    ('mentorship.max_capacity.pro',                   to_jsonb(25),   now()),
    ('mentorship.max_capacity.business',              to_jsonb(50),   now())
ON CONFLICT (key) DO NOTHING;

COMMIT;
