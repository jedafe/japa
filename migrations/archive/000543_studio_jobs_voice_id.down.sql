BEGIN;

DROP INDEX IF EXISTS idx_studio_jobs_voice_id;

ALTER TABLE studio_jobs
    DROP COLUMN IF EXISTS voice_id;

COMMIT;
