-- Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
-- Trigram indexes backing /members/by-stage?q= substring search over
-- username, display_name and profession (ILIKE '%term%' → bitmap scan).
CREATE INDEX IF NOT EXISTS idx_users_username_trgm
    ON users USING gin (username gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_user_profiles_display_name_trgm
    ON user_profiles USING gin (display_name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_user_profiles_profession_trgm
    ON user_profiles USING gin (profession gin_trgm_ops);
