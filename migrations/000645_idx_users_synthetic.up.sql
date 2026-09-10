-- §106 / ADR-117: partial index for looking up synthetic channel users
-- during video import and claim flows.
CREATE INDEX IF NOT EXISTS idx_users_synthetic
    ON users (tenant_id, is_synthetic)
    WHERE is_synthetic = true;
