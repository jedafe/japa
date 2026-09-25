-- 000308_create_visa_timeline_tables.down.sql
BEGIN;
DROP TABLE IF EXISTS visa_processing_stats;
DROP TABLE IF EXISTS visa_timeline_milestones;
DROP TABLE IF EXISTS visa_timelines;
DROP FUNCTION IF EXISTS date_utc(timestamptz);
COMMIT;
