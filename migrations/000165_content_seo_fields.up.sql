-- Section 14: Content SEO fields
-- Adds per-content SEO overrides (meta title/description, OG image, index
-- toggle) so the /create Settings/SEO panel can store author-supplied values
-- instead of falling back to the content title. featured_image_id already
-- exists on the table; this migration only adds the SEO columns.
--
-- All columns are nullable/defaulted so existing rows and existing
-- POST/PATCH callers keep working without changes (Render-time resolution:
-- empty meta_title → fall back to title, empty og_image_url → fall back to
-- featured_image_url, see seo.sql GetPageMetadataBySlug).

ALTER TABLE contents
    ADD COLUMN IF NOT EXISTS meta_title       text,
    ADD COLUMN IF NOT EXISTS meta_description text,
    ADD COLUMN IF NOT EXISTS og_image_url     text,
    ADD COLUMN IF NOT EXISTS seo_indexable    boolean NOT NULL DEFAULT true;
