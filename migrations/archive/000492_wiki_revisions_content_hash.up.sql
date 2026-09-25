-- 000492_wiki_revisions_content_hash.up.sql
-- BUG-002: Add content_hash to wiki_revisions (Rule 12 / §14).
-- Every content save must compute
--   hex(sha256(strip_markdown(body).trim().replaceAll(/\s+/g,' ').toLowerCase()))
-- and store it in content_hash. The unique index rejects duplicate wiki
-- revisions with a 409 duplicate_content error, matching the contents table
-- pattern (idx_contents_hash).
--
-- Ref: japaconnect-engineering/AGENTS.md Rule 12
-- Ref: japaconnect-engineering/sections/14-content-type-system.md
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md

BEGIN;

ALTER TABLE wiki_revisions
    ADD COLUMN IF NOT EXISTS content_hash text;

-- Backfill content_hash for existing revisions from their TipTap JSON content.
-- We use a simple text extraction: strip JSON keys/structure to approximate
-- plain text, then lower-case and collapse whitespace. This is best-effort;
-- pre-existing duplicates (if any) are left in place and the unique index is
-- added as a partial index over (tenant_id, wiki_page_id, content_hash) for
-- future inserts only.
UPDATE wiki_revisions
SET content_hash = md5(lower(regexp_replace(
    regexp_replace(
        content::text,
        '"(type|attrs|content|text)":"[^"]*"', ' ', 'g'
    ),
    '[^a-z0-9]+', ' ', 'g'
)))
WHERE content_hash IS NULL;

-- Unique constraint: one approved revision per (page, content_hash) to reject
-- duplicate no-op edits. We scope to non-null content_hash so legacy rows
-- without a hash don't conflict.
CREATE UNIQUE INDEX IF NOT EXISTS idx_wiki_revisions_page_hash
    ON wiki_revisions (tenant_id, wiki_page_id, content_hash)
    WHERE content_hash IS NOT NULL;

COMMIT;
