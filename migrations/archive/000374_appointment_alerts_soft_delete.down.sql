-- 000374_appointment_alerts_soft_delete.down.sql
BEGIN;

DROP INDEX IF EXISTS idx_appointment_alerts_deleted_at;
ALTER TABLE appointment_alerts DROP COLUMN IF EXISTS deleted_at;

COMMIT;
