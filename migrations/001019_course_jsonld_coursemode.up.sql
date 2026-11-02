-- 001019_course_jsonld_coursemode.up.sql
-- Fixes the seeded Course JSON-LD template's courseMode value. Schema.org /
-- Google enumerate lowercase mode values (online|onsite|hybrid); the seeded
-- template emitted "Online", which validators treat as an unrecognized
-- CourseInstance mode. The template body is admin-editable — this only
-- rewrites the seeded casing, preserving any other admin edits.

BEGIN;

UPDATE content_type_jsonld_templates
SET template = replace(template::text, '"courseMode": "Online"', '"courseMode": "online"')::jsonb,
    updated_at = now()
WHERE content_type = 'Course'
  AND template::text LIKE '%"courseMode": "Online"%';

COMMIT;
