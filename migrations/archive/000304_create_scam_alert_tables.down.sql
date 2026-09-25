-- 000304_create_scam_alert_tables.down.sql
BEGIN;
DROP TABLE IF EXISTS scam_alert_subscriptions;
DROP TABLE IF EXISTS scam_reports;
COMMIT;
