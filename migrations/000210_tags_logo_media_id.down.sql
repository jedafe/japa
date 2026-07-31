-- Down migration for tags logo_media_id (Section 08).
ALTER TABLE tags DROP COLUMN IF EXISTS logo_media_id;
