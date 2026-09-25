-- 000634_contents_video_source_id.up.sql
-- Ref: .jeos/sections/106-video-content-and-import.md
-- Links a content post to its canonical video source.

ALTER TABLE contents
    ADD COLUMN video_source_id uuid REFERENCES video_sources(id) ON DELETE SET NULL;

CREATE INDEX idx_contents_video_source ON contents (video_source_id) WHERE video_source_id IS NOT NULL;
