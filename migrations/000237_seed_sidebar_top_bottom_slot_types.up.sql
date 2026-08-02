-- Add sidebar_top and sidebar_bottom slot types referenced by widget config.
-- The sidebar widget defaults to slot=sidebar_top but this slot didn't exist
-- in the original seed data (000099) or the additional slots migration (000196).
INSERT INTO ad_slot_types (tenant_id, slot_name, description, max_display_per_page) VALUES
    ('00000000-0000-0000-0000-000000000001', 'sidebar_top', 'Top sidebar ad position on main app', 1),
    ('00000000-0000-0000-0000-000000000001', 'sidebar_bottom', 'Bottom sidebar ad position on main app', 1)
ON CONFLICT (tenant_id, slot_name) DO NOTHING;
