BEGIN;

-- 000675_cleanup_video_intel_sources.up.sql
-- §106 — the video service auto-registered every resolved video and every
-- direct upload as an intel_sources row (source_type='video_upload'), so the
-- /sources directory listed individual watch URLs and same-origin media
-- serve paths instead of pull sources. Video sources are now registered at
-- the channel level only (source_type='video_channel', YouTube channel/user
-- URLs); uploads register nothing.
--
-- Delete the polluted per-video/per-upload rows. video_sources.source_id is
-- FK'd ON DELETE SET NULL, so video sources lose the registry link cleanly
-- and re-link to the channel row the next time the channel is registered.
-- Ref: japaconnect-engineering/sections/106-video-content-and-import.md

DELETE FROM intel_sources WHERE source_type = 'video_upload';

COMMIT;
