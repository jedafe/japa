UPDATE public.intel_sources
SET crawl_frequency = 'weekly',
    crawl_frequency_mins = 10080,
    provider_config = provider_config - 'deterministic'
WHERE slug = 'hipolabs-university-list';
