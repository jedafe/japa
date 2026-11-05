-- 001060_staff_quota_overrides.up.sql
-- Seeds staff-tier quota overrides ({staff,admin,super_admin}) for the
-- per-user quota families that previously only had flat keys. Tier-scoped
-- lookups were added in code (tiers.QuotaLimit); these rows give admins
-- explicit tunable values. Only super_admin is unlimited (-1).

BEGIN;

INSERT INTO system_settings (key, value, updated_at) VALUES
    ('api.keys.max_per_user.staff',                 to_jsonb(25),   now()),
    ('api.keys.max_per_user.admin',                 to_jsonb(50),   now()),
    ('api.keys.max_per_user.super_admin',           to_jsonb(-1),   now()),
    ('affiliate.max_links_per_user.staff',          to_jsonb(50),   now()),
    ('affiliate.max_links_per_user.admin',          to_jsonb(100),  now()),
    ('affiliate.max_links_per_user.super_admin',    to_jsonb(-1),   now()),
    ('cv_formatter.quota_standard.staff',           to_jsonb(25),   now()),
    ('cv_formatter.quota_standard.admin',           to_jsonb(50),   now()),
    ('cv_formatter.quota_standard.super_admin',     to_jsonb(-1),   now()),
    ('agent.rate_limit_per_day.staff',              to_jsonb(250),  now()),
    ('agent.rate_limit_per_day.admin',              to_jsonb(500),  now()),
    ('agent.rate_limit_per_day.super_admin',        to_jsonb(-1),   now()),
    ('ai.byok.daily_query_cap.staff',               to_jsonb(2000), now()),
    ('ai.byok.daily_query_cap.admin',               to_jsonb(5000), now()),
    ('ai.byok.daily_query_cap.super_admin',         to_jsonb(-1),   now()),
    ('japa_stage.max_journeys.staff',               to_jsonb(50),   now()),
    ('japa_stage.max_journeys.admin',               to_jsonb(100),  now()),
    ('japa_stage.max_journeys.super_admin',         to_jsonb(-1),   now()),
    ('referral.max_rewards_per_period.staff',       to_jsonb(50),   now()),
    ('referral.max_rewards_per_period.admin',       to_jsonb(100),  now()),
    ('referral.max_rewards_per_period.super_admin', to_jsonb(-1),   now()),
    ('mentorship.max_capacity.staff',               to_jsonb(50),   now()),
    ('mentorship.max_capacity.admin',               to_jsonb(100),  now()),
    ('mentorship.max_capacity.super_admin',         to_jsonb(-1),   now())
ON CONFLICT (key) DO NOTHING;

COMMIT;
