-- 000833: Academy rename cleanup — residual artifacts missed by 000800.
--
-- 1. sidebar_widgets.widget_type: the registry type was renamed
--    learning_enrollments → academy_enrollments. Repoint seeded/admin rows.
-- 2. bookmark_folders: the academy_track default folder was seeded as "learn";
--    rename it "academy" for users whose folder still carries the seed name.
-- 3. Table comment drift: academy_instructor_profiles still describes itself
--    as "Japa University creator identity".

-- ── 1. Sidebar widget type ───────────────────────────────────────────────────
UPDATE public.sidebar_widgets
SET widget_type = 'academy_enrollments'
WHERE widget_type = 'learning_enrollments';

-- ── 2. Default bookmark folder ───────────────────────────────────────────────
-- Only rename folders that still look like the seed default: name = 'learn',
-- the user has no existing 'academy' folder (else the rename would create a
-- confusing duplicate), and every bookmark inside is an academy_track. A user
-- who created a personal folder literally named 'learn' for other content is
-- left untouched.
UPDATE public.bookmark_folders bf
SET name = 'academy'
WHERE bf.name = 'learn'
  AND NOT EXISTS (
      SELECT 1 FROM public.bookmark_folders bf2
      WHERE bf2.user_id = bf.user_id AND bf2.name = 'academy'
  )
  AND NOT EXISTS (
      SELECT 1 FROM public.bookmarks b
      WHERE b.folder_id = bf.id AND b.entity_type <> 'academy_track'
  );

-- ── 3. Table comments ────────────────────────────────────────────────────────
COMMENT ON TABLE public.academy_instructor_profiles
    IS 'Japa Academy creator identity; gated application/approval (ADR-063).';
