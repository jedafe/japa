-- 000840_sidebar_app_check_learn_cleanup.up.sql
--
-- Sidebar audit fixes W1–W4 (Section 59, ADR-066).
--
-- W1 (critical): the 000735 baseline CHECK allowed ('main','docs','learn');
-- 000800 renamed the Learn surface to Academy and migrated live rows, but any
-- row still carrying app='learn' (older snapshot, manual insert) now fails
-- the intended contract. Absorb stragglers (app + slug + name) and enforce
-- the CHECK as ('main','docs','academy'). The 000735 seed rows
-- learn-default-right / learn-lesson-right are the academy-default-right /
-- academy-lesson-right rows after 000800; this UPDATE is a no-op for them
-- and only touches leftovers.
--
-- W2: same absorption covers the legacy 'learn' alias — no separate step.
-- The API keeps a runtime 'learn'→'academy' alias (see Resolve handler and
-- the sidebars service normalizeApp) for old clients.
--
-- W3: delete the leftover 'main-test-left' probe row (slug match; its
-- widgets cascade via sidebar_blocks_sidebar_id_fkey).
--
-- W4: normalize the over-broad '/search*' glob to '/search/*'. Exact
-- '/search' stays covered by the generic main-default-right ('*') fallback.

BEGIN;

ALTER TABLE public.sidebars DROP CONSTRAINT IF EXISTS sidebars_app_check;

-- W1+W2: absorb any legacy 'learn' rows (idempotent after 000800).
UPDATE public.sidebars
SET app = 'academy',
    slug = replace(slug, 'learn-', 'academy-'),
    name = replace(name, 'Learn', 'Academy')
WHERE app = 'learn';

-- W3: remove the test left-rail probe row.
DELETE FROM public.sidebars
WHERE app = 'main' AND position = 'left' AND slug = 'main-test-left';

-- W4: '/search*' matches '/searchanything' — scope to the search subtree.
UPDATE public.sidebars
SET page_pattern = '/search/*'
WHERE page_pattern = '/search*';

ALTER TABLE public.sidebars
    ADD CONSTRAINT sidebars_app_check
    CHECK ((app = ANY (ARRAY['main'::text, 'docs'::text, 'academy'::text])));

COMMIT;
