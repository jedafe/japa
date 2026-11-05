-- 001063_service_listing_quota.up.sql
-- Seeds the per-user service listing quota: flat default plus all six
-- tier variants for services.listings.max_per_user.

BEGIN;

INSERT INTO system_settings (key, value, updated_at) VALUES
    ('services.listings.max_per_user',             to_jsonb(5),   now()),
    ('services.listings.max_per_user.regular',     to_jsonb(5),   now()),
    ('services.listings.max_per_user.pro',         to_jsonb(15),  now()),
    ('services.listings.max_per_user.business',    to_jsonb(50),  now()),
    ('services.listings.max_per_user.staff',       to_jsonb(100), now()),
    ('services.listings.max_per_user.admin',       to_jsonb(250), now()),
    ('services.listings.max_per_user.super_admin', to_jsonb(-1),  now())
ON CONFLICT (key) DO NOTHING;

COMMIT;
