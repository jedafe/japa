-- 000522_add_content_autoassign_lower_indexes.up.sql
-- Adds functional B-tree indexes used by FindSpacesByKeywords for content
-- auto-assignment. The query matches title n-grams against lower(space.name)
-- and lower(custom_field_values.value_text) (university initials).
--
-- Ref: japaconnect-engineering/sections/14-content-type-system.md
--      (Content Auto-Assignment to Spaces)

CREATE INDEX IF NOT EXISTS idx_spaces_name_lower
ON spaces (tenant_id, lower(name))
WHERE deleted_at IS NULL
  AND visibility = 'public'
  AND status = 'active';

CREATE INDEX IF NOT EXISTS idx_cfv_value_text_lower
ON custom_field_values (field_id, lower(value_text))
WHERE entity_type = 'space';
