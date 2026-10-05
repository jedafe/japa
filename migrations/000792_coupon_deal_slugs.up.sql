-- 000792_coupon_deal_slugs.up.sql
--
-- Adds coupons.slug (NOT NULL, UNIQUE) — the canonical URL segment for the
-- public deal pages /deal/{provider-slug}/{coupon-slug} on the main app.
--
-- Backfill derives slugs from title using the same normalization as
-- content.Slugify (lowercase, runs of non [a-z0-9-] → '-', collapse '-',
-- trim leading/trailing '-', empty → 'untitled'), disambiguating collisions
-- with -2, -3, ... suffixes in creation order.
--
-- Ref: .jeos/sections/55-coupons-external-affiliates.md (Deal Pages)

BEGIN;

ALTER TABLE coupons ADD COLUMN slug text;

WITH normalized AS (
    SELECT id, created_at,
           COALESCE(NULLIF(
               regexp_replace(
                 regexp_replace(
                   regexp_replace(lower(title), '[^a-z0-9-]+', '-', 'g'),
                   '-{2,}', '-', 'g'),
                 '^-+|-+$', '', 'g'),
               ''), 'untitled') AS base
    FROM coupons
),
numbered AS (
    SELECT id, base,
           ROW_NUMBER() OVER (PARTITION BY base ORDER BY created_at, id) AS rn
    FROM normalized
)
UPDATE coupons c
SET slug = n.base || CASE WHEN n.rn > 1 THEN '-' || n.rn ELSE '' END
FROM numbered n
WHERE c.id = n.id;

ALTER TABLE coupons ALTER COLUMN slug SET NOT NULL;

CREATE UNIQUE INDEX idx_coupons_slug ON coupons (slug);

COMMIT;
