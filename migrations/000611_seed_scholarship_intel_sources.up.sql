-- 000611_seed_scholarship_intel_sources.up.sql
-- Seeds the initial scholarship intelligence sources for the hybrid
-- Scholarship Finder pipeline. The sources are the two approved API sources
-- (Open Scholarships / Grudged and Grants.gov) and the three approved static
-- HTML sources (DAAD, Erasmus Mundus, and Chevening).
--
-- Ref: docs/superpowers/specs/2026-09-03-scholarship-intelligence-sources-design.md
--      ADR-073, ADR-112

BEGIN;

INSERT INTO intel_sources (
    tenant_id,
    name,
    domain,
    source_url,
    category,
    source_type,
    trust_level,
    crawl_method,
    crawl_frequency,
    crawl_frequency_mins,
    robots_policy,
    source_license,
    attribution_required,
    usage_restriction,
    provider_config,
    enabled,
    status,
    persona_id
)
SELECT
    '00000000-0000-0000-0000-000000000001',
    name,
    domain,
    source_url,
    'scholarship',
    source_type,
    trust_level,
    crawl_method,
    crawl_frequency,
    crawl_frequency_mins,
    '{}'::jsonb,
    source_license,
    true,
    NULL,
    provider_config,
    true,
    'active',
    (SELECT ap.id
     FROM ai_personas ap
     JOIN users u ON u.id = ap.user_id
     WHERE ap.tenant_id = '00000000-0000-0000-0000-000000000001'
       AND u.email = 'intel-scholarship@bot.internal'
     LIMIT 1)
FROM (VALUES
    (
        'Open Scholarships',
        'scholarships.grudged.io',
        'https://scholarships.grudged.io/api/scholarships?availability=open',
        'scholarship_database',
        2,
        'api',
        'daily',
        1440,
        'CC-BY-4.0',
        '{"endpoint":"https://scholarships.grudged.io/api/scholarships?availability=open","method":"GET"}'::jsonb
    ),
    (
        'Grants.gov Search',
        'api.grants.gov',
        'https://api.grants.gov/v1/api/search2',
        'government',
        1,
        'api',
        'daily',
        1440,
        'public-domain',
        '{"endpoint":"https://api.grants.gov/v1/api/search2","method":"POST","body":{"oppStatuses":"posted","rows":100,"keyword":"scholarship education"},"path":"data.opportunityData.opportunityDetails"}'::jsonb
    ),
    (
        'DAAD Scholarship Database',
        'www2.daad.de',
        'https://www2.daad.de/deutschland/stipendium/datenbank/en/21148-scholarship-database/',
        'government',
        1,
        'static',
        'weekly',
        10080,
        'public-domain',
        '{}'::jsonb
    ),
    (
        'Erasmus Mundus Joint Masters',
        'erasmus-plus.ec.europa.eu',
        'https://erasmus-plus.ec.europa.eu/opportunities/individuals/students/erasmus-mundus-joint-masters',
        'international_org',
        1,
        'static',
        'weekly',
        10080,
        'public-domain',
        '{}'::jsonb
    ),
    (
        'Chevening Scholarships',
        'www.chevening.org',
        'https://www.chevening.org/scholarships/',
        'government',
        1,
        'static',
        'weekly',
        10080,
        'public-domain',
        '{}'::jsonb
    )
) AS t(name, domain, source_url, source_type, trust_level, crawl_method, crawl_frequency, crawl_frequency_mins, source_license, provider_config)
ON CONFLICT (tenant_id, source_url) DO UPDATE SET
    name = EXCLUDED.name,
    domain = EXCLUDED.domain,
    category = EXCLUDED.category,
    source_type = EXCLUDED.source_type,
    trust_level = EXCLUDED.trust_level,
    crawl_method = EXCLUDED.crawl_method,
    crawl_frequency = EXCLUDED.crawl_frequency,
    crawl_frequency_mins = EXCLUDED.crawl_frequency_mins,
    robots_policy = EXCLUDED.robots_policy,
    source_license = COALESCE(EXCLUDED.source_license, intel_sources.source_license),
    attribution_required = EXCLUDED.attribution_required,
    usage_restriction = COALESCE(EXCLUDED.usage_restriction, intel_sources.usage_restriction),
    provider_config = EXCLUDED.provider_config,
    enabled = EXCLUDED.enabled,
    status = EXCLUDED.status,
    persona_id = COALESCE(EXCLUDED.persona_id, intel_sources.persona_id),
    updated_at = now();

COMMIT;
