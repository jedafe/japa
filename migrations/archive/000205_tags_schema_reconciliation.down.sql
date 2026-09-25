-- Down migration for 000205_tags_schema_reconciliation.

DROP TRIGGER IF EXISTS trg_tags_content_count_delete ON content_tags;
DROP TRIGGER IF EXISTS trg_tags_content_count_insert ON content_tags;
DROP FUNCTION IF EXISTS fn_tags_content_count_sync();

DROP INDEX IF EXISTS idx_tags_tenant;
CREATE INDEX idx_tags_tenant ON tags (tenant_id);

ALTER TABLE tags DROP COLUMN IF EXISTS field_values;
ALTER TABLE tags DROP COLUMN IF EXISTS is_system;
ALTER TABLE tags DROP COLUMN IF EXISTS content_count;
ALTER TABLE tags DROP COLUMN IF EXISTS cover_media_id;
ALTER TABLE tags DROP COLUMN IF EXISTS rules;
