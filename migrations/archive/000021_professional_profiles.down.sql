-- Down migration for 000021_professional_profiles.sql
-- Drops professional profile invitations, members, profiles, and profile types tables;
-- removes author_profile_id column from contents.
ALTER TABLE contents DROP COLUMN IF EXISTS author_profile_id;
DROP TABLE IF EXISTS professional_profile_invitations;
DROP TABLE IF EXISTS professional_profile_members;
DROP TABLE IF EXISTS professional_profiles;
DROP TABLE IF EXISTS professional_profile_types;
