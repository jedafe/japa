-- Down migration for 000053_mentorship.sql
-- Drops mentorship matches and programs tables.

DROP TABLE IF EXISTS mentorship_matches;
DROP TABLE IF EXISTS mentorship_programs;
