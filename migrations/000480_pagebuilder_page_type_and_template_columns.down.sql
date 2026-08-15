-- 000480_pagebuilder_page_type_and_template_columns.down.sql
-- Reverts 000480: restores the original page_type CHECK constraint
-- (page/sidebar/landing) and drops the four columns added to
-- pagebuilder_templates.
--
-- Ref: japaconnect-engineering/sections/16-page-builder.md

BEGIN;

-- Restore the pre-000480 page_type CHECK (page/sidebar/landing only).
ALTER TABLE pages DROP CONSTRAINT IF EXISTS pages_page_type_check;
ALTER TABLE pages ADD CONSTRAINT pages_page_type_check
    CHECK (page_type IN ('page', 'sidebar', 'landing'));

-- Drop the columns added in 000480.
DROP INDEX IF EXISTS idx_pagebuilder_templates_active;
ALTER TABLE pagebuilder_templates
    DROP COLUMN IF EXISTS is_active,
    DROP COLUMN IF EXISTS thumbnail_media_id,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS schema_version;

COMMIT;
