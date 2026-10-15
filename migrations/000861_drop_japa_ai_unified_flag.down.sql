-- Restore the retired rollout flag (disabled) so a rollback can re-mount
-- the flag dispatch if the paired application release is rolled back.
INSERT INTO feature_flags (key, enabled, created_at)
SELECT 'feature.japa_ai_unified', false, now()
WHERE NOT EXISTS (SELECT 1 FROM feature_flags WHERE key = 'feature.japa_ai_unified');
