-- Space tips: §05 addendum — 'space' recipient kind for member-created custom spaces.
ALTER TABLE tip_settings DROP CONSTRAINT tip_settings_owner_type_check;
ALTER TABLE tip_settings ADD CONSTRAINT tip_settings_owner_type_check
  CHECK (owner_type IN ('user','organization','platform','space'));

ALTER TABLE tips ADD COLUMN IF NOT EXISTS recipient_space_id uuid REFERENCES spaces(id);

ALTER TABLE tips DROP CONSTRAINT tips_recipient_type_check;
ALTER TABLE tips DROP CONSTRAINT tips_recipient_check;
ALTER TABLE tips ADD CONSTRAINT tips_recipient_type_check
  CHECK (recipient_type IN ('platform','user','organization','space'));
ALTER TABLE tips ADD CONSTRAINT tips_recipient_check CHECK (
  (recipient_type = 'platform'     AND recipient_user_id IS NULL     AND recipient_organization_id IS NULL AND recipient_space_id IS NULL) OR
  (recipient_type = 'user'         AND recipient_user_id IS NOT NULL AND recipient_organization_id IS NULL AND recipient_space_id IS NULL) OR
  (recipient_type = 'organization' AND recipient_user_id IS NULL     AND recipient_organization_id IS NOT NULL AND recipient_space_id IS NULL) OR
  (recipient_type = 'space'        AND recipient_user_id IS NOT NULL AND recipient_organization_id IS NULL AND recipient_space_id IS NOT NULL));

CREATE INDEX IF NOT EXISTS idx_tips_space ON tips (recipient_space_id, created_at DESC);
