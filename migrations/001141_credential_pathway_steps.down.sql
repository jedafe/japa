-- 001141_credential_pathway_steps.down.sql

BEGIN;

ALTER TABLE credential_pathways
    DROP COLUMN IF EXISTS pathway_steps;

COMMIT;
