-- Add ON DELETE SET NULL to feature_flags.updated_by FK.
-- Per ADR-057, when a user is hard-deleted (GDPR erasure), the
-- feature_flags.updated_by reference must be nullified rather than
-- blocking the DELETE with a FK violation. The original migration
-- (000007) created the FK with no ON DELETE clause (defaults to
-- NO ACTION).
ALTER TABLE feature_flags DROP CONSTRAINT IF EXISTS feature_flags_updated_by_fkey;

ALTER TABLE feature_flags
    ADD CONSTRAINT feature_flags_updated_by_fkey
    FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL;
