-- 000751_settings_seeds.up.sql
-- Seed system_settings keys introduced by the §80 rollup work and the
-- intel rejected-record retention sweep. Code falls back to these same
-- defaults via GetIntOr when the rows are absent; the seeds exist so the
-- keys are admin-editable from the settings UI (per the
-- system-settings-catalog convention: keys with defaults must be seeded).
--
-- Ref: .jeos/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    ('wiki.rollup_default_rows_limit', to_jsonb(50)),
    ('intel.rejected_retention_days', to_jsonb(7))
ON CONFLICT (key) DO NOTHING;

COMMIT;
