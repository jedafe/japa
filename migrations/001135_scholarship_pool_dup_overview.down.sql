-- Restore the scholarships -> wiki_scholarship_overview pool row at its
-- original position (1).

BEGIN;

INSERT INTO comparison_type_field_groups (type_id, group_id, category, label, position, is_default)
SELECT td.id, g.id, 'overview', 'Overview', 1, false
FROM comparison_type_definitions td
JOIN custom_field_groups g ON g.slug = 'wiki_scholarship_overview'
WHERE td.slug = 'scholarships'
ON CONFLICT DO NOTHING;

COMMIT;
