DROP INDEX IF EXISTS idx_contents_federate_opt_out;
ALTER TABLE contents DROP COLUMN IF EXISTS federate_opt_out;
