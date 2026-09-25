ALTER TABLE space_members ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'approved';
