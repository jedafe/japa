-- 000439_seed_persona_max_batch_size.up.sql
-- Seed the persona.max_batch_size system setting (Rule 3: configurable, not
-- hardcoded). Caps the number of personas a single POST /admin/personas
-- request can provision so an admin cannot submit an unbounded batch that
-- would hold a long transaction and DoS the writer pool (BUG-006).
-- Default 100 is a sensible cap for a single admin action; tenants can tune
-- it via system_settings without a code change.

BEGIN;

INSERT INTO system_settings (tenant_id, key, value)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'persona.max_batch_size', '"100"')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
