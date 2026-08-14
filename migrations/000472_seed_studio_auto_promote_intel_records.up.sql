-- 000472_seed_studio_auto_promote_intel_records.up.sql
-- Seeds the studio.auto_promote_intel_records system setting (ADR-076
-- Amendment 1). When true, high-confidence intel_records are automatically
-- promoted to Studio jobs for editorial generation. Defaults to false —
-- a human must explicitly promote extracted records via the admin
-- Intelligence review UI until the operator opts in.

BEGIN;

INSERT INTO system_settings (tenant_id, key, value)
VALUES ('00000000-0000-0000-0000-000000000001', 'studio.auto_promote_intel_records', '"false"')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
