-- Tags logo support (Section 08).
-- Adds logo_media_id to the tags table so each tag can have a logo image
-- (distinct from cover_media_id which is a banner/cover). The logo is
-- displayed on tag cards and the tag hub page in the main frontend, and
-- is uploadable via the admin tag create/edit form.
-- Ref: japaconnect-engineering/sections/08-forum-qa-solved.md

ALTER TABLE tags ADD COLUMN IF NOT EXISTS logo_media_id uuid REFERENCES media(id) ON DELETE SET NULL;
