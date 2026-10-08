-- 000810_marketplace_content_fields.up.sql
-- Services Marketplace follow-ups (§114): typed faqs[]/steps[] content fields
-- on service_listing_meta plus the badge-derivation + content-cap settings.
-- Ref: .jeos/sections/114-services-marketplace.md
BEGIN;

ALTER TABLE service_listing_meta
    ADD COLUMN IF NOT EXISTS faqs  jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(faqs) = 'array'),
    ADD COLUMN IF NOT EXISTS steps jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(steps) = 'array');

-- settings (code defaults identical)
INSERT INTO system_settings (key, value) VALUES
    ('service_marketplace.content.max_faqs',   to_jsonb(12)),
    ('service_marketplace.content.max_steps',  to_jsonb(8)),
    ('service_marketplace.badges.min_reviews', to_jsonb(10)),
    ('service_marketplace.badges.min_rating',  to_jsonb(4.0))
ON CONFLICT (key) DO NOTHING;

COMMIT;
