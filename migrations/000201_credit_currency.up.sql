-- 000201_credit_currency.up.sql
-- Section 58: Credit Currency System (ADR-065)
-- Three-piece architecture: currencies reference table + spaces.currency_code pointer + credit_packs

-- =====================
-- Currencies reference table
-- =====================
CREATE TABLE currencies (
    code             text PRIMARY KEY,
    name             text NOT NULL,
    symbol           text NOT NULL,
    credits_per_unit int NOT NULL CHECK (credits_per_unit > 0),
    is_active        boolean NOT NULL DEFAULT true,
    position         int NOT NULL DEFAULT 0,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

-- Seed the initial supported currencies.
-- Rates are fixed (no live FX). Admin can edit via Control Panel.
-- Example: 1 USD = 1400 credits, 1 NGN = 1 credit, 1 ZAR = 75 credits.
INSERT INTO currencies (code, name, symbol, credits_per_unit, position) VALUES
    ('NGN', 'Nigerian Naira',    '₦',   1,    1),
    ('USD', 'US Dollar',         '$',   1400, 2),
    ('GBP', 'British Pound',     '£',   1750, 3),
    ('EUR', 'Euro',              '€',   1500, 4),
    ('ZAR', 'South African Rand','R',   75,   5),
    ('CAD', 'Canadian Dollar',   'C$',  1000, 6),
    ('AED', 'UAE Dirham',        'د.إ', 380,  7),
    ('GHS', 'Ghanaian Cedi',     '₵',   110,  8)
ON CONFLICT (code) DO NOTHING;

-- =====================
-- Country → currency linkage on spaces
-- =====================
ALTER TABLE spaces ADD COLUMN currency_code text REFERENCES currencies(code) ON DELETE SET NULL;

-- Backfill the 11 seeded country spaces (migration 000100_seed_geo).
UPDATE spaces SET currency_code = 'NGN' WHERE geo_code = 'NG' AND level = 'country';
UPDATE spaces SET currency_code = 'GBP' WHERE geo_code = 'GB' AND level = 'country';
UPDATE spaces SET currency_code = 'CAD' WHERE geo_code = 'CA' AND level = 'country';
UPDATE spaces SET currency_code = 'USD' WHERE geo_code = 'US' AND level = 'country';
UPDATE spaces SET currency_code = 'EUR' WHERE geo_code = 'IE' AND level = 'country';
UPDATE spaces SET currency_code = 'EUR' WHERE geo_code = 'DE' AND level = 'country';
UPDATE spaces SET currency_code = 'EUR' WHERE geo_code = 'NL' AND level = 'country';
UPDATE spaces SET currency_code = 'ZAR' WHERE geo_code = 'ZA' AND level = 'country';
UPDATE spaces SET currency_code = 'AED' WHERE geo_code = 'AE' AND level = 'country';
UPDATE spaces SET currency_code = 'GHS' WHERE geo_code = 'GH' AND level = 'country';
UPDATE spaces SET currency_code = 'EUR' WHERE geo_code = 'FR' AND level = 'country';

-- =====================
-- User preferred currency (override)
-- =====================
ALTER TABLE user_profiles ADD COLUMN preferred_currency text REFERENCES currencies(code) ON DELETE SET NULL;

-- =====================
-- Credit packs (admin-defined bundles with optional bonus)
-- =====================
CREATE TABLE credit_packs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            text NOT NULL,
    description     text,
    credit_amount   int NOT NULL CHECK (credit_amount > 0),
    bonus_credits   int NOT NULL DEFAULT 0 CHECK (bonus_credits >= 0),
    min_user_level  text,
    is_active       boolean NOT NULL DEFAULT true,
    position        int NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);
CREATE INDEX idx_credit_packs_tenant_active ON credit_packs(tenant_id, is_active, position);

-- =====================
-- System settings seed
-- =====================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'wallet.default_currency', '"NGN"'),
    ('00000000-0000-0000-0000-000000000001', 'wallet.credit_purchase_enabled', 'true')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================
-- RBAC permission nodes
-- =====================
INSERT INTO permissions (key, description) VALUES
    ('wallet.currency.manage', 'Manage currencies and exchange rates (Control Panel)'),
    ('wallet.credit_pack.manage', 'Manage credit packs (Admin Panel)'),
    ('wallet.purchases.view_any', 'View any user credit purchase history (admin)')
ON CONFLICT (key) DO NOTHING;

-- Grant wallet.currency.manage to super_admin only
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('wallet.currency.manage', 'wallet.credit_pack.manage', 'wallet.purchases.view_any')
ON CONFLICT DO NOTHING;

-- Grant wallet.credit_pack.manage and wallet.purchases.view_any to admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'admin'
  AND p.key IN ('wallet.credit_pack.manage', 'wallet.purchases.view_any')
ON CONFLICT DO NOTHING;

-- Grant wallet.credits.purchase to member (already exists in seed, ensure granted)
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'wallet.credits.purchase'
ON CONFLICT DO NOTHING;
