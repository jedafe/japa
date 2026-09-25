-- 000633_video_user_columns.up.sql
-- Ref: .jeos/sections/106-video-content-and-import.md
-- Columns to represent synthetic video authors/channels and their public channel metadata.

ALTER TABLE users
    ADD COLUMN is_synthetic boolean NOT NULL DEFAULT false,
    ADD COLUMN synthetic_source text;

ALTER TABLE user_profiles
    ADD COLUMN location text,
    ADD COLUMN banner_media_id uuid REFERENCES media(id) ON DELETE SET NULL;
