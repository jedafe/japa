-- Seed the Hipolabs university dataset as a disabled 'university' intel
-- source (crawl_method=api). §39 already uses Hipolabs for the CLI seeder;
-- this row lets an admin deliberately enable the continuous-ingestion
-- path. Disabled by default: a full run extracts ~9k records.
INSERT INTO public.intel_sources (
    name, domain, source_url, category, source_type, trust_level,
    crawl_method, crawl_frequency, crawl_frequency_mins,
    provider_config, enabled, status, source_kind, slug,
    description, categories
) VALUES (
    'Hipolabs University List',
    'universities.hipolabs.com',
    'http://universities.hipolabs.com/search',
    'university',
    'web',
    2,
    'api',
    'weekly',
    10080,
    '{"method": "GET", "endpoint": "http://universities.hipolabs.com/search", "entity_types": ["university"]}'::jsonb,
    false,
    'active',
    'aggregator',
    'hipolabs-university-list',
    'Free JSON directory of ~9,000 universities worldwide (name, domains, country, alpha_two_code). Enable to bulk-ingest universities through the intel pipeline.',
    '{university}'
);
