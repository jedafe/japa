-- 000754_lesson_progress_position
-- Adds position_secs to lesson_progress so POST /learn/lessons/:id/progress
-- can persist a video playback position (the Learn frontend sends
-- progress_secs every ~15s while a video lesson plays; §36 "player with
-- progress_secs tracking"). NULL means no saved position — distinct from 0.

BEGIN;

ALTER TABLE lesson_progress
    ADD COLUMN position_secs integer;

COMMIT;
