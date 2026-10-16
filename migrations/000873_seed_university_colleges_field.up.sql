-- 000873: add 'colleges' to university_fields — populated by
-- wiki:populate_facts from Wikidata P527 (has part) with constituent
-- colleges/faculties, kept distinct from programs_offered which lists
-- actual course offerings from university_course_offerings.
INSERT INTO custom_fields (id, group_id, label, "key", field_type, options, embed_platforms, placeholder, default_value, is_required, is_system, show_in_frontend, filterable, sortable, searchable, "position", created_at, config) VALUES
('00000000-0000-0000-0000-000000000105', '00000000-0000-0000-0000-000000000044', 'Colleges / Faculties', 'colleges', 'text', NULL, NULL, NULL, NULL, false, true, true, false, false, false, 17, now(), '{}')
ON CONFLICT (id) DO NOTHING;
