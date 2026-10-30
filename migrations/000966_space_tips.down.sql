DROP INDEX IF EXISTS idx_tips_space;
ALTER TABLE tips DROP CONSTRAINT IF EXISTS tips_recipient_check;
ALTER TABLE tips DROP COLUMN IF EXISTS recipient_space_id;
ALTER TABLE tips DROP CONSTRAINT tips_recipient_type_check;
ALTER TABLE tips ADD CONSTRAINT tips_recipient_type_check
  CHECK (recipient_type IN ('platform','user','organization'));
ALTER TABLE tips ADD CONSTRAINT tips_recipient_check CHECK (
  (recipient_type = 'platform'     AND recipient_user_id IS NULL     AND recipient_organization_id IS NULL) OR
  (recipient_type = 'user'         AND recipient_user_id IS NOT NULL AND recipient_organization_id IS NULL) OR
  (recipient_type = 'organization' AND recipient_user_id IS NULL     AND recipient_organization_id IS NOT NULL));
ALTER TABLE tip_settings DROP CONSTRAINT tip_settings_owner_type_check;
ALTER TABLE tip_settings ADD CONSTRAINT tip_settings_owner_type_check
  CHECK (owner_type IN ('user','organization','platform'));
