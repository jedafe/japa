-- 000575_jce_ingestion_jobs.up.sql
-- JCE Sub-spec 4: audit/status table for comparison value refresh runs.
-- Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md

BEGIN;

CREATE TABLE IF NOT EXISTS comparison_data_ingestion_jobs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id      uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed','cancelled')),
    source_kind     text,
    refreshed_count int NOT NULL DEFAULT 0,
    error           text,
    started_at      timestamptz,
    completed_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comp_ingestion_jobs_tenant_content
    ON comparison_data_ingestion_jobs(tenant_id, content_id, created_at DESC);

COMMIT;
