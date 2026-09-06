-- 000617_dedupe_route_sidebar_widgets.down.sql
-- The up migration removed exact-duplicate widget rows. Duplicated rows are
-- indistinguishable and cannot be restored, so this down migration is a
-- documented no-op. Re-run the affected seed INSERTs (000221–000224) to
-- repopulate widgets if a rollback is ever required.

BEGIN;

SELECT 1;

COMMIT;
