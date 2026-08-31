-- 000580_jce_gdpr_owners.up.sql
-- JCE Sub-spec 4: add ownership columns for GDPR erasure/export.
-- comparison_data_ingestion_jobs currently links to user content; the
-- comparison_data_sources table needs a created_by_user_id column so future
-- non-admin created sources can be deleted on account erasure.
-- Ref: .jeos/audit/jce-audit.md (JCE-2026-08-31-008)

BEGIN;

ALTER TABLE comparison_data_sources
    ADD COLUMN IF NOT EXISTS created_by_user_id uuid DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_comparison_data_sources_created_by
    ON comparison_data_sources(tenant_id, created_by_user_id)
    WHERE created_by_user_id IS NOT NULL;

COMMIT;
