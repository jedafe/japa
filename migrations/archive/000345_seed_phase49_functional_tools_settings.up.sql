-- 000345_seed_phase49_functional_tools_settings.up.sql
-- Seeds system_settings for all V2 Functional Tools (Phase 49).
-- §90 Visa Routes, §91 Appointment Alerter, §92 Arrival Checklist,
-- §94 Credential Checker, §95 CV Formatter, §96 Housing Guide,
-- §97 Attestation Guide, §99 Language Test Hub, §100 Tax Calculator,
-- §101 Community Locator.
-- §93 Processing Time Tracker reuses §83 visa_timeline settings.
-- Ref: japaconnect-engineering/sections/90-97,99-101

INSERT INTO system_settings (tenant_id, key, value) VALUES
    -- §90 Visa Route Comparison
    ('00000000-0000-0000-0000-000000000001', 'visa_routes.enabled',          'false'),
    ('00000000-0000-0000-0000-000000000001', 'visa_routes.subdomain',        '""'),
    ('00000000-0000-0000-0000-000000000001', 'visa_routes.custom_domain',    '""'),
    -- §91 Embassy Appointment Alerter
    ('00000000-0000-0000-0000-000000000001', 'appointment_alert.enabled',    'false'),
    ('00000000-0000-0000-0000-000000000001', 'appointment_alert.min_poll_seconds', '300'),
    -- §92 Arrival Checklist
    ('00000000-0000-0000-0000-000000000001', 'arrival_checklist.enabled',    'false'),
    ('00000000-0000-0000-0000-000000000001', 'arrival_checklist.subdomain',  '""'),
    ('00000000-0000-0000-0000-000000000001', 'arrival_checklist.custom_domain', '""'),
    -- §94 Professional Credential Checker
    ('00000000-0000-0000-0000-000000000001', 'credential_checker.enabled',   'false'),
    ('00000000-0000-0000-0000-000000000001', 'credential_checker.subdomain', '""'),
    ('00000000-0000-0000-0000-000000000001', 'credential_checker.custom_domain', '""'),
    -- §95 CV Formatter
    ('00000000-0000-0000-0000-000000000001', 'cv_formatter.enabled',         'false'),
    ('00000000-0000-0000-0000-000000000001', 'cv_formatter.subdomain',       '""'),
    ('00000000-0000-0000-0000-000000000001', 'cv_formatter.custom_domain',   '""'),
    ('00000000-0000-0000-0000-000000000001', 'cv_formatter.quota_standard',  '3'),
    -- §96 Housing Guide
    ('00000000-0000-0000-0000-000000000001', 'housing_guide.enabled',        'false'),
    ('00000000-0000-0000-0000-000000000001', 'housing_guide.subdomain',      '""'),
    ('00000000-0000-0000-0000-000000000001', 'housing_guide.custom_domain',  '""'),
    -- §97 Attestation Guide
    ('00000000-0000-0000-0000-000000000001', 'attestation_guide.enabled',    'false'),
    ('00000000-0000-0000-0000-000000000001', 'attestation_guide.subdomain',  '""'),
    ('00000000-0000-0000-0000-000000000001', 'attestation_guide.custom_domain', '""'),
    -- §99 Language Test Hub
    ('00000000-0000-0000-0000-000000000001', 'language_test_hub.enabled',    'false'),
    ('00000000-0000-0000-0000-000000000001', 'language_test_hub.subdomain',  '""'),
    ('00000000-0000-0000-0000-000000000001', 'language_test_hub.custom_domain', '""'),
    -- §100 Tax Calculator
    ('00000000-0000-0000-0000-000000000001', 'tax_calculator.enabled',       'false'),
    ('00000000-0000-0000-0000-000000000001', 'tax_calculator.subdomain',     '""'),
    ('00000000-0000-0000-0000-000000000001', 'tax_calculator.custom_domain', '""'),
    -- §101 Diaspora Community Locator
    ('00000000-0000-0000-0000-000000000001', 'community_locator.enabled',    'false'),
    ('00000000-0000-0000-0000-000000000001', 'community_locator.subdomain',  '""'),
    ('00000000-0000-0000-0000-000000000001', 'community_locator.custom_domain', '""')
ON CONFLICT (tenant_id, key) DO NOTHING;
