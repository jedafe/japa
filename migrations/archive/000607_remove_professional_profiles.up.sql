-- 000607_remove_professional_profiles.up.sql
-- Remove Professional Profiles and reparent dependent data.

BEGIN;

-- Remove author_profile_id columns from contents and comments.
ALTER TABLE contents DROP COLUMN IF EXISTS author_profile_id;
ALTER TABLE comments DROP COLUMN IF EXISTS author_profile_id;

-- Remove the FK and null out ai_personas.profile_id.
ALTER TABLE ai_personas DROP CONSTRAINT IF EXISTS ai_personas_profile_id_fkey;
ALTER TABLE ai_personas ALTER COLUMN profile_id DROP NOT NULL;
UPDATE ai_personas SET profile_id = NULL;

-- Convert professional_profile follows to user follows, pointing to the owner.
UPDATE follows
SET entity_type = 'user',
    entity_id = (SELECT owner_user_id FROM professional_profiles WHERE id = entity_id)
WHERE entity_type = 'professional_profile';

-- Deduplicate follows that may now point to a user the follower already follows.
DELETE FROM follows a
USING follows b
WHERE a.id < b.id
  AND a.tenant_id = b.tenant_id
  AND a.follower_id = b.follower_id
  AND a.entity_type = b.entity_type
  AND a.entity_id = b.entity_id;

-- Drop columns on other tables that reference professional profiles.
ALTER TABLE professional_relocations DROP COLUMN IF EXISTS profile_id;
ALTER TABLE professional_documents DROP COLUMN IF EXISTS profile_id;
ALTER TABLE professional_verifiers DROP COLUMN IF EXISTS profile_id;

-- Drop professional profile tables.
DROP TABLE IF EXISTS professional_profile_invitations;
DROP TABLE IF EXISTS professional_profile_members;
DROP TABLE IF EXISTS professional_profiles;
DROP TABLE IF EXISTS professional_profile_types;

COMMIT;
