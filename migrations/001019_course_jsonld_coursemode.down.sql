-- 001019_course_jsonld_coursemode.down.sql
-- Restores the pre-migration casing of the seeded Course JSON-LD
-- template's courseMode value.

BEGIN;

UPDATE content_type_jsonld_templates
SET template = replace(template::text, '"courseMode": "online"', '"courseMode": "Online"')::jsonb,
    updated_at = now()
WHERE content_type = 'Course'
  AND template::text LIKE '%"courseMode": "online"%';

COMMIT;
