-- 000717_tools_is_japabot_indexed_generated.up.sql
-- Convert is_japabot_indexed on the §§107–110 SEO growth tool tables to
-- GENERATED ALWAYS AS columns per the BUG-011 precedent (migration
-- 000375). The plain boolean columns defaulted false and were never
-- updated, which would have silently excluded every tool document from
-- the Meilisearch RAG indexes (ADR-088).
--
-- Expression: status = 'published' AND is_active (where present).
-- crs_draws and sop_samples have no is_active column.
--
-- Ref: japaconnect-engineering/sections/107-crs-calculator.md
--      japaconnect-engineering/sections/108-pof-calculator.md
--      japaconnect-engineering/sections/109-eca-guide.md
--      japaconnect-engineering/sections/110-sop-generator.md

BEGIN;

ALTER TABLE crs_rulesets DROP COLUMN is_japabot_indexed;
ALTER TABLE crs_rulesets ADD COLUMN is_japabot_indexed BOOLEAN GENERATED ALWAYS AS (
    status = 'active'
) STORED;

ALTER TABLE crs_draws DROP COLUMN is_japabot_indexed;
ALTER TABLE crs_draws ADD COLUMN is_japabot_indexed BOOLEAN GENERATED ALWAYS AS (
    status = 'published'
) STORED;

ALTER TABLE pof_requirements DROP COLUMN is_japabot_indexed;
ALTER TABLE pof_requirements ADD COLUMN is_japabot_indexed BOOLEAN GENERATED ALWAYS AS (
    status = 'published' AND is_active = TRUE
) STORED;

ALTER TABLE eca_guides DROP COLUMN is_japabot_indexed;
ALTER TABLE eca_guides ADD COLUMN is_japabot_indexed BOOLEAN GENERATED ALWAYS AS (
    status = 'published' AND is_active = TRUE
) STORED;

ALTER TABLE sop_samples DROP COLUMN is_japabot_indexed;
ALTER TABLE sop_samples ADD COLUMN is_japabot_indexed BOOLEAN GENERATED ALWAYS AS (
    status = 'published'
) STORED;

COMMIT;
