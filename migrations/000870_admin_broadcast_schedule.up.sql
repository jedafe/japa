-- §35 scheduled broadcasts: a row with scheduled_at set and sent_at NULL
-- is pending; the broadcast:dispatch_scheduled Asynq sweep delivers it and
-- stamps sent_at. target/target_id persist the audience scope chosen at
-- create time (all | user_group | geo_space).
ALTER TABLE admin_broadcasts
    ADD COLUMN scheduled_at timestamptz,
    ADD COLUMN target       text        NOT NULL DEFAULT 'all',
    ADD COLUMN target_id    uuid,
    ALTER COLUMN sent_at    DROP NOT NULL,
    ALTER COLUMN sent_at    DROP DEFAULT;

-- partially-covered pending scan (tiny: pending broadcasts are rare)
CREATE INDEX idx_admin_broadcasts_pending
    ON admin_broadcasts (scheduled_at)
    WHERE scheduled_at IS NOT NULL AND sent_at IS NULL;
