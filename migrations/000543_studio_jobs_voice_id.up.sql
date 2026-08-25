-- ADR-114: per-job persona voice override for AI Content Studio.
BEGIN;

ALTER TABLE studio_jobs
    ADD COLUMN IF NOT EXISTS voice_id uuid REFERENCES persona_voices(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_studio_jobs_voice_id
    ON studio_jobs (tenant_id, voice_id)
    WHERE voice_id IS NOT NULL;

COMMIT;
