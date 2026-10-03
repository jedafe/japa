-- 000780_fx_rates.up.sql
-- §112 / ADR-125: mid-market FX matrix + remittance benchmark provider flag.
-- (Planned as 000777; number taken by travel seeds — next free is 000780.)
--
-- Ref: .jeos/sections/112-fx-converter.md
--      .jeos/adr/ADR-125-fx-converter.md

BEGIN;

CREATE TABLE fx_rates (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    base_currency  text NOT NULL,
    quote_currency text NOT NULL,
    rate           numeric(18,6) NOT NULL,
    source         text NOT NULL DEFAULT 'oxr',
    fetched_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fx_rates_latest
    ON fx_rates (base_currency, quote_currency, fetched_at DESC);

ALTER TABLE remittance_providers
    ADD COLUMN is_benchmark boolean NOT NULL DEFAULT false;

-- Seeded benchmark provider. Inactive until an admin pastes an OXR App ID
-- into api_config via PATCH /api/v1/admin/remittance/providers/:id — the
-- handler encrypts api_config on write (ADR-075 §1).
-- remittance_providers_slug_key is a UNIQUE index — conflict-safe insert.
INSERT INTO remittance_providers
    (name, slug, website_url, data_source, api_config, is_active, is_benchmark, display_order)
VALUES
    ('Mid-market (Open Exchange Rates)', 'mid-market', 'https://openexchangerates.org',
     'api', NULL, false, true, 0)
ON CONFLICT (slug) DO NOTHING;

-- §112 system_settings (catalog: conventions/system-settings-catalog.md).
-- value is jsonb: numbers are bare JSON, strings are double-quoted.
-- fx.subdomain / fx.custom_domain are is_public=true so /site/settings/public
-- exposes them to apps/tools host detection (ADR-086).
INSERT INTO system_settings (key, value, is_public) VALUES
    ('fx.cache_ttl_minutes', '120', false),
    ('fx.stale_hours', '48', false),
    ('fx.history_retention_days', '90', false),
    ('fx.default_base_currency', '"USD"', false),
    ('fx.subdomain', '"currency-converter"', true),
    ('fx.custom_domain', '""', true)
ON CONFLICT (key) DO NOTHING;

COMMIT;
