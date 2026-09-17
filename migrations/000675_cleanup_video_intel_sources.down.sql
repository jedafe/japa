BEGIN;

-- 000675_cleanup_video_intel_sources.down.sql
-- The deleted intel_sources rows were per-video/per-upload registrations that
-- cannot be reconstructed (title and URL came from live resolve metadata).
-- Re-resolving a video re-registers its channel under source_type
-- 'video_channel'. No rollback action.

COMMIT;
