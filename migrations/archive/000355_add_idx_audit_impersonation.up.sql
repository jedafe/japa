-- 000355_add_idx_audit_impersonation.up.sql
-- Adds the missing idx_audit_impersonation partial index required by
-- Section 46 (Admin Impersonation) and ADR-047. Supports the control-panel
-- audit log explorer's ?impersonation_only=true filter without a seq scan.

CREATE INDEX IF NOT EXISTS idx_audit_impersonation
    ON audit_logs (impersonated_user_id, created_at DESC)
    WHERE impersonated_user_id IS NOT NULL;
