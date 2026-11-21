-- 001224_housing_guides_status_default.up.sql
-- Section 96 (Housing Guide) + ADR-088 (HITL).
--
-- housing_guides.status DEFAULT was 'published' (baseline schema, shared
-- with the other tool tables). Every code path sends status explicitly —
-- CreateGuide defaults to 'draft' — so the column default only fires for
-- raw INSERTs and seeds, where it silently lands a publicly visible row
-- with no HITL review stamps. The 001223 CHECK constrains values to the
-- enum but does not close this: 'published' is a valid enum value.
-- Default to 'draft' so an unspecified insert is review-gated instead of
-- live.
--
-- Ref: japaconnect-engineering/sections/96-housing-guide.md

ALTER TABLE housing_guides ALTER COLUMN status SET DEFAULT 'draft';
