-- 001141_credential_pathway_steps.up.sql
-- Credential Checker (§94) — structured pathway_steps mirroring
-- eca_guides.pathway_steps ([{step, title, detail, duration}]) so the
-- public tool renders a real step-by-step pathway instead of prose only.
--
-- Ref: japaconnect-engineering/sections/94-professional-credential-checker.md

BEGIN;

ALTER TABLE credential_pathways
    ADD COLUMN IF NOT EXISTS pathway_steps jsonb NOT NULL DEFAULT '[]'::jsonb;

COMMIT;
