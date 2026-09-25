-- Revert ON DELETE SET NULL back to the original NO ACTION behaviour.
ALTER TABLE feature_flags DROP CONSTRAINT IF EXISTS feature_flags_updated_by_fkey;

ALTER TABLE feature_flags
    ADD CONSTRAINT feature_flags_updated_by_fkey
    FOREIGN KEY (updated_by) REFERENCES users(id);
