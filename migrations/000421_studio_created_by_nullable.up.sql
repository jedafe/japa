-- 000421_studio_created_by_nullable.up.sql
-- Makes studio_jobs.created_by and studio_batches.created_by nullable so the
-- GDPR erasure flow can nullify them when an admin who created studio jobs is
-- deleted. Without this, the final HardDeleteUser blocks on the residual NOT
-- NULL foreign key (Rule 11, §15 GDPR erasure model, ADR-014).
--
-- Ref: japaconnect-engineering/sections/75-ai-content-studio.md
-- Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md

BEGIN;

ALTER TABLE studio_jobs    ALTER COLUMN created_by DROP NOT NULL;
ALTER TABLE studio_batches ALTER COLUMN created_by DROP NOT NULL;

COMMIT;
