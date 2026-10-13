-- 000840_sidebar_app_check_learn_cleanup.down.sql
--
-- Revert the W1–W4 cleanup: drop the academy CHECK and restore the 000800
-- post-state CHECK ('main','docs','learn'). Data reverts are best-effort —
-- the deleted test row is NOT restored and '/search/*' is kept (both were
-- probe/over-broad values with no restore contract).

BEGIN;

ALTER TABLE public.sidebars DROP CONSTRAINT IF EXISTS sidebars_app_check;

UPDATE public.sidebars
SET app = 'learn',
    slug = replace(slug, 'academy-', 'learn-'),
    name = replace(name, 'Academy', 'Learn')
WHERE app = 'academy';

ALTER TABLE public.sidebars
    ADD CONSTRAINT sidebars_app_check
    CHECK ((app = ANY (ARRAY['main'::text, 'docs'::text, 'learn'::text])));

COMMIT;
