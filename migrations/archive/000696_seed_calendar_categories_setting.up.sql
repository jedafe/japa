-- 000696_seed_calendar_categories_setting.up.sql
-- Renumbered from 000458 — it collided with 000458_grant_bookmarks_manage_member_plus
-- and 000458_reseed_blog_edit_permission_nodes. De-tenanted form:
-- system_settings.tenant_id was dropped by 000668 and rows are keyed globally on
-- `key`. Tail-position body; databases that applied the 000458 file skip this
-- via the 000000 registry marker.

INSERT INTO system_settings (key, value) VALUES
    ('calendar.categories',
     '[
        {"value":"visa_window","label":"Visa window"},
        {"value":"embassy_closure","label":"Embassy closure"},
        {"value":"application_deadline","label":"Application deadline"},
        {"value":"exam_window","label":"Exam window"},
        {"value":"immigration_office","label":"Immigration office"},
        {"value":"other","label":"Other"}
     ]'::jsonb)
ON CONFLICT (key) DO NOTHING;
