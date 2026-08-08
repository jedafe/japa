-- Rollback: drop the feature_flags table.
-- This destroys all tenant feature flag overrides; the registry defaults
-- in internal/features/registry.go take over after rollback.
DROP TABLE IF EXISTS feature_flags;
