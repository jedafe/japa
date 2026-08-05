-- 000268 — Allow page_type='landing' in the pages table.
--
-- The landing-page builder (admin /landing route, Section 16 page builder) creates
-- and lists pages with page_type='landing'. Migration 000217 originally seeded the
-- check constraint with only ('page', 'sidebar'), so every POST /admin/pages with
-- page_type='landing' failed with a constraint violation (500). This extends the
-- constraint to include 'landing'.
--
-- Ref: japaconnect-engineering/sections/16-page-builder.md

ALTER TABLE pages DROP CONSTRAINT IF EXISTS pages_page_type_check;
ALTER TABLE pages ADD CONSTRAINT pages_page_type_check
    CHECK (page_type IN ('page', 'sidebar', 'landing'));
