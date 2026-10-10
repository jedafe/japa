-- 000829_service_detail_page_content.down.sql
-- Reverse of 000829 up, in reverse order.
BEGIN;

DELETE FROM system_settings WHERE key IN (
    'service_marketplace.content.max_facts',
    'service_marketplace.content.max_selling_points',
    'service_marketplace.content.max_included',
    'service_marketplace.content.max_highlights',
    'service_marketplace.content.max_audience'
);

ALTER TABLE user_profiles
    DROP COLUMN IF EXISTS mentor_success_label,
    DROP COLUMN IF EXISTS mentor_years_experience,
    DROP COLUMN IF EXISTS mentor_credentials,
    DROP COLUMN IF EXISTS mentor_countries,
    DROP COLUMN IF EXISTS mentor_session_formats,
    DROP COLUMN IF EXISTS mentor_languages,
    DROP COLUMN IF EXISTS mentor_session_type,
    DROP COLUMN IF EXISTS mentor_availability,
    DROP COLUMN IF EXISTS mentor_response_time,
    DROP COLUMN IF EXISTS mentor_title;

ALTER TABLE service_listing_meta
    DROP COLUMN IF EXISTS included,
    DROP COLUMN IF EXISTS audience,
    DROP COLUMN IF EXISTS highlights,
    DROP COLUMN IF EXISTS facts,
    DROP COLUMN IF EXISTS selling_points;

COMMIT;
