-- 000458_seed_calendar_categories_setting.up.sql
-- BUG-009: move the hardcoded calendar category enum from the frontend into
-- a system_settings key so it is admin-configurable and served via the API
-- (Rule 3: no hardcoded config; Rule 16: frontend consumes API).
-- The category column on calendar_entries is TEXT (not a pg_enum), so the
-- allowed values are enforced at the service layer. This setting is the
-- canonical source of the allowed list.
-- Ref: japaconnect-engineering/sections/86-community-calendar.md
-- Ref: japaconnect-engineering/AGENTS.md Rule 3, Rule 16

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001',
     'calendar.categories',
     '[
        {"value":"visa_window","label":"Visa window"},
        {"value":"embassy_closure","label":"Embassy closure"},
        {"value":"application_deadline","label":"Application deadline"},
        {"value":"exam_window","label":"Exam window"},
        {"value":"immigration_office","label":"Immigration office"},
        {"value":"other","label":"Other"}
     ]'::jsonb)
ON CONFLICT (tenant_id, key) DO NOTHING;
