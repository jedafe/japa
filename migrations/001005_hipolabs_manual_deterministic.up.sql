-- Flip the seeded Hipolabs source to manual crawl + the deterministic
-- (LLM-free) extractor: bulk refresh on demand, no token cost.
UPDATE public.intel_sources
SET crawl_frequency = 'manual',
    crawl_frequency_mins = 0,
    provider_config = provider_config || '{"deterministic": "hipolabs"}'::jsonb
WHERE slug = 'hipolabs-university-list';
