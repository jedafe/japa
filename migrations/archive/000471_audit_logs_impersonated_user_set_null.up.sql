-- GDPR: change audit_logs.impersonated_user_id FK to ON DELETE SET NULL
-- so that GDPR user erasure (HardDeleteUser) is not blocked by audit log
-- rows referencing the deleted user. The audit log row itself is preserved
-- (it is the canonical compliance record) with impersonated_user_id set to
-- NULL — the actor_user_id still identifies the admin who performed the
-- action. Section 46 specifies ON DELETE SET NULL; the original migration
-- (000027_gdpr_compliance.sql) created the FK without the clause.
-- Ref: japaconnect-engineering/sections/46-impersonation.md (Audit Log
-- Schema Extension) + sections/15-gdpr.md.

ALTER TABLE audit_logs DROP CONSTRAINT IF EXISTS audit_logs_impersonated_user_id_fkey;
ALTER TABLE audit_logs
    ADD CONSTRAINT audit_logs_impersonated_user_id_fkey
    FOREIGN KEY (impersonated_user_id) REFERENCES users(id) ON DELETE SET NULL;
