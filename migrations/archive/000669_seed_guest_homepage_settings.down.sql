DELETE FROM system_settings WHERE key IN (
    'landing.stats',
    'landing.categories',
    'landing.featured_opportunities',
    'landing.top_destinations'
);
