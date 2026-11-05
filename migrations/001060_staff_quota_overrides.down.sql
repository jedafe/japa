-- 001060_staff_quota_overrides.down.sql
-- Removes the staff-tier quota override keys seeded by 001060.

BEGIN;

DELETE FROM system_settings WHERE key IN (
    'api.keys.max_per_user.staff',
    'api.keys.max_per_user.admin',
    'api.keys.max_per_user.super_admin',
    'affiliate.max_links_per_user.staff',
    'affiliate.max_links_per_user.admin',
    'affiliate.max_links_per_user.super_admin',
    'cv_formatter.quota_standard.staff',
    'cv_formatter.quota_standard.admin',
    'cv_formatter.quota_standard.super_admin',
    'agent.rate_limit_per_day.staff',
    'agent.rate_limit_per_day.admin',
    'agent.rate_limit_per_day.super_admin',
    'ai.byok.daily_query_cap.staff',
    'ai.byok.daily_query_cap.admin',
    'ai.byok.daily_query_cap.super_admin',
    'japa_stage.max_journeys.staff',
    'japa_stage.max_journeys.admin',
    'japa_stage.max_journeys.super_admin',
    'referral.max_rewards_per_period.staff',
    'referral.max_rewards_per_period.admin',
    'referral.max_rewards_per_period.super_admin',
    'mentorship.max_capacity.staff',
    'mentorship.max_capacity.admin',
    'mentorship.max_capacity.super_admin'
);

COMMIT;
