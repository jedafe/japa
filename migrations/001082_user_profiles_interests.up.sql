-- Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
-- Free-form interest/skill chips on profiles — shown on japa-stage directory
-- cards and matched by /members/by-stage?q=. Edited via PATCH /me/profile.
ALTER TABLE user_profiles ADD COLUMN interests text[];
