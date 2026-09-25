-- ADR-064: Geo-Aware Content Personalization
-- The only schema change required. Everything else (VisitorContext resolver,
-- feed boost, Meilisearch weighting, default-to-ContentCountry on ?geo= routes)
-- is application logic + config; trending_snapshots.geo_scope (000-series) and
-- GeoContext (Section 37) already exist.

ALTER TABLE user_profiles
    ADD COLUMN IF NOT EXISTS content_country_preference text;  -- ISO code; NULL = use resolved VisitorContext.ContentCountry

COMMENT ON COLUMN user_profiles.content_country_preference IS
    'User override for geo content weighting (ISO country code). NULL = use resolved VisitorContext (ADR-064).';
