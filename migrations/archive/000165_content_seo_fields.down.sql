-- Reverse of 000165_content_seo_fields.up.sql.
ALTER TABLE contents
    DROP COLUMN IF EXISTS seo_indexable,
    DROP COLUMN IF EXISTS og_image_url,
    DROP COLUMN IF EXISTS meta_description,
    DROP COLUMN IF EXISTS meta_title;
