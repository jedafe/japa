-- Sidebar Management System (Section 59, ADR-066): remove seeded widget rows
-- whose widget types are route-owned and can never render from the sidebar
-- system. `table_of_contents` is rendered by the docs page route snippet and
-- `curriculum_rail` by the learn lesson player layout — both resolved to
-- permanent no-ops in the widget renderer. `continue_learning` is main-only in
-- the registry (learn renders its hardcoded ContinueLearningCard per Section
-- 59), so the learn-seeded rows are dead config.
DELETE FROM public.sidebar_widgets
WHERE widget_type IN ('curriculum_rail', 'table_of_contents');

DELETE FROM public.sidebar_widgets
WHERE widget_type = 'continue_learning'
  AND sidebar_id IN (SELECT id FROM public.sidebars WHERE app = 'learn');
