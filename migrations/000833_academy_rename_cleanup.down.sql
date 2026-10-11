-- 000833 (down): revert Academy rename cleanup.

UPDATE public.sidebar_widgets
SET widget_type = 'learning_enrollments'
WHERE widget_type = 'academy_enrollments';

UPDATE public.bookmark_folders bf
SET name = 'learn'
WHERE bf.name = 'academy'
  AND NOT EXISTS (
      SELECT 1 FROM public.bookmark_folders bf2
      WHERE bf2.user_id = bf.user_id AND bf2.name = 'learn'
  )
  AND NOT EXISTS (
      SELECT 1 FROM public.bookmarks b
      WHERE b.folder_id = bf.id AND b.entity_type <> 'academy_track'
  );

COMMENT ON TABLE public.academy_instructor_profiles
    IS 'Japa University creator identity; gated application/approval (ADR-063).';
