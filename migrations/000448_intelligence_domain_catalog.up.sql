-- 000448_intelligence_domain_catalog.up.sql
-- Seeds i18n catalog entries and system settings for the new intelligence
-- domains added by the expansion: housing, cost_of_living, travel_routes,
-- grants, admission, banking, work_career, health_insurance,
-- culture_lifestyle.
--
-- The existing migration 000438 already seeds scholarship, job, remittance,
-- visa, fx, news, travel_alert. This migration adds the remaining guide
-- content type categories so the admin UI can display localized labels
-- for source categories and the radar can label items by domain.
--
-- Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 8)

BEGIN;

-- English labels for the new source categories.
INSERT INTO translations (tenant_id, locale, "key", value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'en', 'intel.category_housing', 'Housing'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'intel.category_cost_of_living', 'Cost of Living'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'intel.category_travel_routes', 'Travel Routes'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'intel.category_grants', 'Grants'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'intel.category_admission', 'Admission'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'intel.category_banking', 'Banking'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'intel.category_work_career', 'Work & Career'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'intel.category_health_insurance', 'Health Insurance'),
    ('00000000-0000-0000-0000-000000000001', 'en', 'intel.category_culture_lifestyle', 'Culture & Lifestyle')
ON CONFLICT (tenant_id, locale, "key") DO NOTHING;

-- French labels for the new source categories.
INSERT INTO translations (tenant_id, locale, "key", value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'fr', 'intel.category_housing', 'Logement'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'intel.category_cost_of_living', 'Coût de la vie'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'intel.category_travel_routes', 'Itinéraires de voyage'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'intel.category_grants', 'Subventions'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'intel.category_admission', 'Admission'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'intel.category_banking', 'Banque'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'intel.category_work_career', 'Travail & Carrière'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'intel.category_health_insurance', 'Assurance santé'),
    ('00000000-0000-0000-0000-000000000001', 'fr', 'intel.category_culture_lifestyle', 'Culture & Mode de vie')
ON CONFLICT (tenant_id, locale, "key") DO NOTHING;

-- System settings for the guide content type domains (freshness decay
-- policy: "slow" for relatively static guide content like culture/lifestyle,
-- "normal" for time-sensitive content like admission deadlines).
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'intel.travel_routes.enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.travel_routes.freshness_decay_policy', '"normal"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.grants.enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.grants.freshness_decay_policy', '"normal"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.admission.enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.admission.freshness_decay_policy', '"normal"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.banking.enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.banking.freshness_decay_policy', '"slow"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.work_career.enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.work_career.freshness_decay_policy', '"normal"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.health_insurance.enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.health_insurance.freshness_decay_policy', '"slow"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.culture_lifestyle.enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.culture_lifestyle.freshness_decay_policy', '"slow"')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
