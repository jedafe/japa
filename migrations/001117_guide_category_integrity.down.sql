-- 001117_guide_category_integrity.down.sql
DELETE FROM content_media_fields WHERE field_key = 'og_image';

DROP TRIGGER IF EXISTS trg_entity_types_guide_category_delete ON custom_field_entity_types;
DROP TRIGGER IF EXISTS trg_contents_guide_category ON contents;
DROP FUNCTION IF EXISTS entity_types_guard_guide_category_delete();
DROP FUNCTION IF EXISTS contents_validate_guide_category();
