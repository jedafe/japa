-- 000829_service_detail_page_content.up.sql
-- Services marketplace detail-page rebuild (§114 2026-10-10 amendment):
-- structured page-content jsonb columns on service_listing_meta for the
-- mockup-aligned single pages (selling-point trust chips, hero fact rows,
-- highlights/audience checklists, what's-included cards), plus the mentor
-- card fields on user_profiles that feed the mentorship page's mentor
-- block, stats row, and "Mentorship details" sidebar.
--
-- All columns are additive with defaults → zero downtime.
-- Ref: .jeos/sections/114-services-marketplace.md

BEGIN;

-- Listing page content — [{icon,title,description?}] / [{icon,label,value}]
-- / [string] shapes; replace-on-save on PATCH like faqs[]/steps[].
ALTER TABLE service_listing_meta
    ADD COLUMN IF NOT EXISTS selling_points jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(selling_points) = 'array'),
    ADD COLUMN IF NOT EXISTS facts jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(facts) = 'array'),
    ADD COLUMN IF NOT EXISTS highlights jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(highlights) = 'array'),
    ADD COLUMN IF NOT EXISTS audience jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(audience) = 'array'),
    ADD COLUMN IF NOT EXISTS included jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(included) = 'array');

-- Mentor card fields — headline card, stats row, and the "Mentorship
-- details" sidebar on listing_kind='mentorship' detail pages. All
-- free-form text/text[]: mentors describe themselves ("Within 24 hours",
-- "Mon – Sat", "95% admission success rate") rather than pick enums.
ALTER TABLE user_profiles
    ADD COLUMN IF NOT EXISTS mentor_title text,
    ADD COLUMN IF NOT EXISTS mentor_response_time text,
    ADD COLUMN IF NOT EXISTS mentor_availability text,
    ADD COLUMN IF NOT EXISTS mentor_session_type text,
    ADD COLUMN IF NOT EXISTS mentor_languages text[],
    ADD COLUMN IF NOT EXISTS mentor_session_formats text[],
    ADD COLUMN IF NOT EXISTS mentor_countries text[],
    ADD COLUMN IF NOT EXISTS mentor_credentials text[],
    ADD COLUMN IF NOT EXISTS mentor_years_experience smallint,
    ADD COLUMN IF NOT EXISTS mentor_success_label text;

-- Content caps — same pattern as max_faqs/max_steps (000810).
INSERT INTO system_settings (key, value) VALUES
    ('service_marketplace.content.max_facts',          to_jsonb(8)),
    ('service_marketplace.content.max_selling_points', to_jsonb(6)),
    ('service_marketplace.content.max_included',       to_jsonb(12)),
    ('service_marketplace.content.max_highlights',     to_jsonb(12)),
    ('service_marketplace.content.max_audience',       to_jsonb(12))
ON CONFLICT (key) DO NOTHING;

COMMIT;
