-- Re-insert the seed rows removed by 000736_sidebar_dead_widget_rows.up.sql.
INSERT INTO public.sidebars (id, app, position, slug, name, page_pattern, visibility, required_group_ids, status, created_at, updated_at)
VALUES
    ('a1b2c3d4-0003-4000-8000-000000000001', 'docs', 'right', 'docs-default-right', 'Docs Default Right', '*', 'all', '[]', 'active', '2026-08-01 02:20:56.729956+01', '2026-08-01 02:20:56.729956+01'),
    ('a1b2c3d4-0004-4000-8000-000000000001', 'learn', 'right', 'learn-default-right', 'Learn Default Right', '*', 'all', '[]', 'active', '2026-08-01 02:20:56.729956+01', '2026-08-01 02:20:56.729956+01'),
    ('a1b2c3d4-0004-4000-8000-000000000002', 'learn', 'right', 'learn-lesson-right', 'Learn Lesson Right', '/tracks/*', 'all', '[]', 'active', '2026-08-01 02:20:56.729956+01', '2026-08-01 02:20:56.729956+01')
ON CONFLICT (id) DO NOTHING;

INSERT INTO public.sidebar_widgets (id, sidebar_id, widget_type, label, props, sort_order, visibility, required_group_ids, permission_node, status, created_at, updated_at)
VALUES
    ('15e389c1-059d-4da3-80e3-1be5628b083e', 'a1b2c3d4-0003-4000-8000-000000000001', 'table_of_contents', 'On this page', '{"title": "On this page"}', 10, 'all', '[]', NULL, 'active', '2026-08-01 03:35:36.327852+01', '2026-08-01 03:35:36.327852+01'),
    ('62d23db5-f579-44f3-acd4-b7a6b00e870d', 'a1b2c3d4-0004-4000-8000-000000000001', 'continue_learning', 'Continue Learning', '{"title": "Continue Learning"}', 9, 'authenticated', '[]', NULL, 'active', '2026-08-01 03:35:36.327852+01', '2026-08-01 03:35:36.327852+01'),
    ('ab19b865-ca58-4542-8139-ab0f6fae9c3f', 'a1b2c3d4-0004-4000-8000-000000000002', 'continue_learning', 'Next Lesson', '{"title": "Continue Learning"}', 10, 'authenticated', '[]', NULL, 'active', '2026-08-01 03:35:36.327852+01', '2026-08-01 03:35:36.327852+01'),
    ('04a55bd1-2926-43f5-82d4-fa453e1ed6cf', 'a1b2c3d4-0004-4000-8000-000000000002', 'curriculum_rail', 'Curriculum', '{}', 11, 'all', '[]', NULL, 'active', '2026-08-01 03:35:36.327852+01', '2026-08-01 03:35:36.327852+01')
ON CONFLICT (id) DO NOTHING;
