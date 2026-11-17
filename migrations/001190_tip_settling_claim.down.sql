-- Rollback note: any row sitting in 'settling' is returned to 'pending'
-- first so the restored check accepts it.
UPDATE tips SET status = 'pending' WHERE status = 'settling';
ALTER TABLE tips DROP CONSTRAINT tips_status_check;
ALTER TABLE tips ADD CONSTRAINT tips_status_check
    CHECK ((status = ANY (ARRAY['pending'::text, 'completed'::text, 'failed'::text, 'refunded'::text])));
