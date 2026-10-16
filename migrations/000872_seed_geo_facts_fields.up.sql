-- 000872: seed the additional geo_fields used by the wiki:populate_facts
-- enrichment task (Wikidata → wiki page custom fields for geo spaces).
-- Group 'geo_fields' (4dfd79c2-…) already has population/area/capital/
-- cost_of_living/language; these add the remaining general-info fields.
-- Field UUIDs share the group's prefix for readability.
INSERT INTO custom_fields (id, group_id, label, "key", field_type, options, embed_platforms, placeholder, default_value, is_required, is_system, show_in_frontend, filterable, sortable, searchable, "position", created_at, config) VALUES
('4dfd79c2-d087-4e5c-80b1-bb10b5f90001', '4dfd79c2-d087-4e5c-80b1-bb10b5f90b89', 'Currency', 'currency', 'text', NULL, NULL, NULL, NULL, false, true, true, false, false, false, 10, now(), '{}'),
('4dfd79c2-d087-4e5c-80b1-bb10b5f90002', '4dfd79c2-d087-4e5c-80b1-bb10b5f90b89', 'Postal Codes', 'postal_codes', 'text', NULL, NULL, NULL, NULL, false, true, true, false, false, false, 11, now(), '{}'),
('4dfd79c2-d087-4e5c-80b1-bb10b5f90003', '4dfd79c2-d087-4e5c-80b1-bb10b5f90b89', 'Calling Code', 'calling_code', 'text', NULL, NULL, NULL, NULL, false, true, true, false, false, false, 12, now(), '{}'),
('4dfd79c2-d087-4e5c-80b1-bb10b5f90004', '4dfd79c2-d087-4e5c-80b1-bb10b5f90b89', 'Official Website', 'official_website', 'text', NULL, NULL, NULL, NULL, false, true, true, false, false, false, 13, now(), '{}'),
('4dfd79c2-d087-4e5c-80b1-bb10b5f90005', '4dfd79c2-d087-4e5c-80b1-bb10b5f90b89', 'Founded / Established', 'founded', 'text', NULL, NULL, NULL, NULL, false, true, true, false, false, false, 14, now(), '{}'),
('4dfd79c2-d087-4e5c-80b1-bb10b5f90006', '4dfd79c2-d087-4e5c-80b1-bb10b5f90b89', 'Head of Government', 'head_of_government', 'text', NULL, NULL, NULL, NULL, false, true, true, false, false, false, 15, now(), '{}'),
('4dfd79c2-d087-4e5c-80b1-bb10b5f90007', '4dfd79c2-d087-4e5c-80b1-bb10b5f90b89', 'Time Zone', 'time_zone', 'text', NULL, NULL, NULL, NULL, false, true, true, false, false, false, 16, now(), '{}')
ON CONFLICT (id) DO NOTHING;
