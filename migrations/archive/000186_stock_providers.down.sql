ALTER TABLE media DROP COLUMN IF EXISTS source_photo_id;
ALTER TABLE media DROP COLUMN IF EXISTS source_license_url;
ALTER TABLE media DROP COLUMN IF EXISTS source_license;
ALTER TABLE media DROP COLUMN IF EXISTS source_photographer_url;
ALTER TABLE media DROP COLUMN IF EXISTS source_photographer;
ALTER TABLE media DROP COLUMN IF EXISTS source_platform;
DROP TABLE IF EXISTS stock_providers;
