-- 000935_verification_fields_id_repair.down.sql
-- Removes the d022-d024 verification fields; leaves readiness fields
-- untouched (their state under a 000933-then-000934 rollback is the
-- pre-feature baseline).

DELETE FROM custom_fields
WHERE id IN (
  '00000000-0000-0000-0000-00000000d022',
  '00000000-0000-0000-0000-00000000d023',
  '00000000-0000-0000-0000-00000000d024'
);
