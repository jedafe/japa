DROP INDEX IF EXISTS idx_admin_broadcasts_pending;
ALTER TABLE admin_broadcasts
    DROP COLUMN scheduled_at,
    DROP COLUMN target,
    DROP COLUMN target_id;
-- restore NOT NULL: any pending rows would violate, so mark them sent
UPDATE admin_broadcasts SET sent_at = now() WHERE sent_at IS NULL;
ALTER TABLE admin_broadcasts
    ALTER COLUMN sent_at SET DEFAULT now(),
    ALTER COLUMN sent_at SET NOT NULL;
