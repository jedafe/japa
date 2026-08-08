-- 000306_create_planning_group_tables.down.sql
BEGIN;
DROP TABLE IF EXISTS planning_group_checkins;
DROP TABLE IF EXISTS planning_group_milestones;
DROP TABLE IF EXISTS planning_group_members;
DROP TABLE IF EXISTS planning_groups;
DROP FUNCTION IF EXISTS week_start_utc(timestamptz);
COMMIT;
