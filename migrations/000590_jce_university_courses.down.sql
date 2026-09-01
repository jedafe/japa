-- Drop the university course catalog and offerings tables.
-- Ref: japaconnect-engineering/sections/39-space-types-university.md
BEGIN;

DROP TABLE IF EXISTS university_course_offerings;
DROP TABLE IF EXISTS university_course_catalog;

COMMIT;
