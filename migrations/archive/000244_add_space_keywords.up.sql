-- Add keywords column to spaces for auto-assigning content based on title matching.
-- Ref: japaconnect-engineering/sections/05-geo-first-space-system.md
-- When a user creates content without selecting a space, the backend searches
-- for primary (country/state/city) and public spaces whose keywords appear in
-- the content title, and auto-assigns the content to those spaces.
-- Keywords are stored as a text array for efficient array overlap queries.

ALTER TABLE spaces ADD COLUMN IF NOT EXISTS keywords text[] NOT NULL DEFAULT '{}';

-- Create a GIN index for fast array overlap queries (&& operator).
CREATE INDEX IF NOT EXISTS idx_spaces_keywords ON spaces USING GIN (keywords);
