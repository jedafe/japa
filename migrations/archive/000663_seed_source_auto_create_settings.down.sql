BEGIN;

DELETE FROM system_settings
WHERE key IN (
    'sources.auto_create_for_universities',
    'sources.auto_create_for_organizations',
    'sources.auto_create_for_users',
    'intel.radar.mixed_fetch_page_multiplier'
);

COMMIT;
