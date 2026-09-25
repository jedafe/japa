-- Rollback: Geo-Aware Content Personalization (ADR-064)

ALTER TABLE user_profiles
    DROP COLUMN IF EXISTS content_country_preference;
