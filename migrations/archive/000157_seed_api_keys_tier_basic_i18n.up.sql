-- Table name corrected from a nonexistent "i18n_keys" to the real
-- translations table (see migrations/000046_i18n.sql) — a typo that only
-- surfaced when replaying every migration against a truly fresh database
-- while building the ADR-061 integration-test harness (Task A0).
INSERT INTO translations (tenant_id, locale, "key", value)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'en', 'api_keys.tier_basic', 'Basic'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'api_keys.tier_basic', 'Basique')
ON CONFLICT (tenant_id, locale, "key") DO UPDATE SET value = EXCLUDED.value;
