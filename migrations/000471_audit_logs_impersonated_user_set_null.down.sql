-- Revert: restore the original FK without ON DELETE SET NULL.
-- Note: this restores the pre-fix constraint shape. If any audit_logs rows
-- have impersonated_user_id = NULL due to a user erasure that happened
-- while this migration was applied, those rows remain valid (the column
-- is nullable).

ALTER TABLE audit_logs DROP CONSTRAINT IF EXISTS audit_logs_impersonated_user_id_fkey;
ALTER TABLE audit_logs
    ADD CONSTRAINT audit_logs_impersonated_user_id_fkey
    FOREIGN KEY (impersonated_user_id) REFERENCES users(id);
