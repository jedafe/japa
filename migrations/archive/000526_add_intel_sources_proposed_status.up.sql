-- 000526_add_intel_sources_proposed_status.up.sql
-- Adds source proposal workflow columns to intel_sources (ADR-112).
--
-- Existing rows become status='active'. New sources discovered by the agent
-- are created with status='proposed' and enabled=false until an admin
-- approves them.
--
-- Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md

BEGIN;

ALTER TABLE intel_sources
    ADD COLUMN status text NOT NULL DEFAULT 'active',
    ADD COLUMN proposed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN proposed_at timestamptz,
    ADD COLUMN approved_by uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN approved_at timestamptz;

COMMENT ON COLUMN intel_sources.status IS 'active | proposed | disabled | rejected';

COMMIT;
