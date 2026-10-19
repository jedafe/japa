-- Backfill is data repair, not schema: down is a no-op. Re-nulling the
-- resolved spaces would only re-break the geo filter.
SELECT 1;
