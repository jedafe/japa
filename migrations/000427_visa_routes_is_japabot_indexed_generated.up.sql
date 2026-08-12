-- 000427_visa_routes_is_japabot_indexed_generated.up.sql
-- Fix: §90 spec requires is_japabot_indexed to be a GENERATED ALWAYS AS
-- (status = 'published' AND is_active = TRUE) STORED computed column, but
-- migration 000325 created it as a plain boolean. This migration drops the
-- plain column and recreates it as a generated column so the RAG-indexability
-- gate is automatically maintained (ADR-088/§69 JapaBot Indexability Pattern).
--
-- Ref: japaconnect-engineering/sections/90-visa-route-comparison.md
--      japaconnect-engineering/adr/ADR-088-destination-wiki.md

BEGIN;

ALTER TABLE visa_routes DROP COLUMN IF EXISTS is_japabot_indexed;

ALTER TABLE visa_routes
    ADD COLUMN is_japabot_indexed BOOLEAN GENERATED ALWAYS AS
        (status = 'published' AND is_active = TRUE) STORED;

COMMIT;
