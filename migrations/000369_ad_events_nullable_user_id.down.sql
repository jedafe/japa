-- 000369 down: Restore NOT NULL on ad_events.user_id.
-- Note: this will fail if any rows with NULL user_id exist. Run
-- DELETE FROM ad_events WHERE user_id IS NULL before applying down.
ALTER TABLE ad_events ALTER COLUMN user_id SET NOT NULL;
