-- Irreversible data repair: the pre-backfill seo_indexable values were a
-- mix of manual toggles, leaked column defaults, and partially-derived
-- writes — there is no single prior state to restore. Correctness does not
-- depend on the backfill anyway: the flag is re-derived on every content
-- write path, so rows converge on their next update. Intentionally a
-- no-op.

BEGIN;
-- no-op
COMMIT;
