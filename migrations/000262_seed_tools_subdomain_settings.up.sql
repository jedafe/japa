-- 000262_seed_tools_subdomain_settings.up.sql
-- Seeds the 12 missing tools web surface subdomain/custom_domain system_settings
-- (ADR-086, §78). The university.* keys were seeded in migration 000169; this
-- migration covers the remaining 6 tools × 2 keys (subdomain + custom_domain).
-- Ref: japaconnect-engineering/conventions/system-settings-catalog.md
-- Ref: japaconnect-engineering/adr/ADR-086-tools-web-surface.md

INSERT INTO system_settings (tenant_id, key, value) VALUES
    -- Tools hub
    ('00000000-0000-0000-0000-000000000001', 'tools.subdomain',           '"tools"'),
    ('00000000-0000-0000-0000-000000000001', 'tools.custom_domain',       '""'),
    -- Remittance Tracker (§74, ADR-075)
    ('00000000-0000-0000-0000-000000000001', 'remittance.subdomain',      '"remittance"'),
    ('00000000-0000-0000-0000-000000000001', 'remittance.custom_domain',  '""'),
    -- Scholarship Finder (§73, ADR-073)
    ('00000000-0000-0000-0000-000000000001', 'scholarship_finder.subdomain',     '"scholarships"'),
    ('00000000-0000-0000-0000-000000000001', 'scholarship_finder.custom_domain', '""'),
    -- Cost of Living Calculator (§68, ADR-070)
    ('00000000-0000-0000-0000-000000000001', 'col_calculator.subdomain',         '"cost-of-living"'),
    ('00000000-0000-0000-0000-000000000001', 'col_calculator.custom_domain',     '""'),
    -- Visa Checklist (§66, ADR-068)
    ('00000000-0000-0000-0000-000000000001', 'checklist.subdomain',        '"visa-checklist"'),
    ('00000000-0000-0000-0000-000000000001', 'checklist.custom_domain',    '""'),
    -- Readiness Assessment (§71, ADR-071)
    ('00000000-0000-0000-0000-000000000001', 'assessment.subdomain',      '"readiness"'),
    ('00000000-0000-0000-0000-000000000001', 'assessment.custom_domain',  '""'),
    -- JapaBot (§77, ADR-074)
    ('00000000-0000-0000-0000-000000000001', 'japabot.subdomain',         '"japabot"'),
    ('00000000-0000-0000-0000-000000000001', 'japabot.custom_domain',     '""')
ON CONFLICT (tenant_id, key) DO NOTHING;
