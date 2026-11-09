-- Rows created with status='published' never pass through UpdateContentStatus,
-- so their published_at stayed NULL — anchor-minted discussion threads,
-- imports, and any other born-published row. created_at is the only timestamp
-- the row actually has, so it's the honest stamp.
UPDATE contents
SET published_at = created_at
WHERE status = 'published' AND published_at IS NULL;
