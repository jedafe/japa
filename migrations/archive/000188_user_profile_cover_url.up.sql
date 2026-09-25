-- Add cover_url to user_profiles for profile cover photos.
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS cover_url TEXT;
