-- 000601_seed_default_credit_packs.up.sql
-- Section 58: seed starter credit packs for the default tenant.

BEGIN;

INSERT INTO credit_packs (
    tenant_id,
    name,
    description,
    credit_amount,
    bonus_credits,
    min_user_level,
    is_active,
    position
)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'Starter', 'A small credit pack to get started.', 100, 0, NULL, true, 1),
    ('00000000-0000-0000-0000-000000000001', 'Pro', 'Best value for active users.', 500, 50, NULL, true, 2),
    ('00000000-0000-0000-0000-000000000001', 'Max', 'Maximum credits with a generous bonus.', 1000, 150, NULL, true, 3)
ON CONFLICT (tenant_id, name) DO NOTHING;

COMMIT;
