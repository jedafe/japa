BEGIN;

ALTER TABLE video_sources DROP COLUMN IF EXISTS source_id;
ALTER TABLE comparison_data_sources DROP COLUMN IF EXISTS source_id;

-- Note: any intel_sources rows created by the up migration remain in place to
-- avoid orphaning content; they can be cleaned up by a later admin if needed.

COMMIT;
