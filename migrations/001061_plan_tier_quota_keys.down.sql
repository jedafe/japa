-- 001061_plan_tier_quota_keys.down.sql
-- Removes the plan-tier quota keys seeded by 001061.

BEGIN;

DELETE FROM system_settings WHERE key IN (
    'api.keys.max_per_user.regular',
    'api.keys.max_per_user.pro',
    'api.keys.max_per_user.business',
    'affiliate.max_links_per_user.regular',
    'affiliate.max_links_per_user.pro',
    'affiliate.max_links_per_user.business',
    'cv_formatter.quota_standard.regular',
    'cv_formatter.quota_standard.pro',
    'cv_formatter.quota_standard.business',
    'agent.rate_limit_per_day.regular',
    'agent.rate_limit_per_day.pro',
    'agent.rate_limit_per_day.business',
    'ai.byok.daily_query_cap.regular',
    'ai.byok.daily_query_cap.pro',
    'ai.byok.daily_query_cap.business',
    'japa_stage.max_journeys.regular',
    'japa_stage.max_journeys.pro',
    'japa_stage.max_journeys.business',
    'referral.max_rewards_per_period.regular',
    'referral.max_rewards_per_period.pro',
    'referral.max_rewards_per_period.business',
    'mentorship.max_capacity.regular',
    'mentorship.max_capacity.pro',
    'mentorship.max_capacity.business'
);

COMMIT;
