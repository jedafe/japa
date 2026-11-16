DROP TABLE IF EXISTS federation_remote_shares;
ALTER TABLE federation_deliveries
    DROP CONSTRAINT IF EXISTS federation_deliveries_activity_inbox_uniq;
