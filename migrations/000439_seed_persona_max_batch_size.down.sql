-- 000439_seed_persona_max_batch_size.down.sql

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'persona.max_batch_size';

COMMIT;
