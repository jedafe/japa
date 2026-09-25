-- 000601_seed_default_credit_packs.down.sql
-- Remove the seeded default credit packs.

BEGIN;

DELETE FROM credit_packs
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name IN ('Starter', 'Pro', 'Max');

COMMIT;
