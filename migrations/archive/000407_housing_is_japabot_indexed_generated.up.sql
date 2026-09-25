-- 000407_housing_is_japabot_indexed_generated.up.sql
-- Fix BUG-011: is_japabot_indexed should be a GENERATED ALWAYS AS column
-- per §96 spec (status = 'published' AND is_active = TRUE) STORED, per
-- ADR-088 JapaBot Indexability Pattern. The original migration (000335)
-- created it as a plain boolean default false, which breaks RAG
-- indexability filtering (Meilisearch sync filters is_japabot_indexed =
-- true).
--
-- Ref: japaconnect-engineering/sections/96-housing-guide.md
--      japaconnect-engineering/skills/geo-aware-tool-pattern.md (JapaBot Indexability Pattern)

BEGIN;

ALTER TABLE housing_guides DROP COLUMN IF EXISTS is_japabot_indexed;

ALTER TABLE housing_guides ADD COLUMN is_japabot_indexed BOOLEAN GENERATED ALWAYS AS (
    status = 'published' AND is_active = TRUE
) STORED;

COMMIT;
