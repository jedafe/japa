-- 000435_add_university_initials_trgm_index.up.sql
-- Adds a partial pg_trgm GIN index on custom_field_values.value_text for the
-- university "initials" field, so list/search queries on /universities can use
-- ILIKE '%query%' on acronyms without scanning the whole table.
--
-- Ref: japaconnect-engineering/sections/39-space-types-university.md

CREATE INDEX IF NOT EXISTS idx_cfv_initials_value_trgm
ON custom_field_values USING gin (value_text gin_trgm_ops)
WHERE entity_type = 'space'
  AND field_id = '00000000-0000-0000-0000-000000000090'::uuid;
