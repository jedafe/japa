-- 000384_comments_body_text_to_jsonb.up.sql
-- Migrates comments.body from plain text to jsonb TipTap JSON (Section 49).
--
-- Existing plain-text rows are backfilled into a minimal TipTap document
-- envelope {type:"doc",content:[{type:"paragraph",content:[{type:"text",
-- text:<body>}]}]} before the column type is changed, so the ALTER succeeds
-- without a per-row cast failure.
--
-- Ref: japaconnect-engineering/sections/49-comments-replies.md

BEGIN;

-- =====================
-- Step 1: backfill existing text bodies into TipTap JSON envelopes.
-- A row is considered "already JSON" if it starts with '{' (a TipTap doc
-- object). Everything else is treated as legacy plain text and wrapped.
-- =====================
UPDATE comments
SET body = jsonb_build_object(
    'type', 'doc',
    'content', jsonb_build_array(
        jsonb_build_object(
            'type', 'paragraph',
            'content', jsonb_build_array(
                jsonb_build_object('type', 'text', 'text', body)
            )
        )
    )
)::text
WHERE body IS NOT NULL
  AND body != ''
  AND body NOT LIKE '{%';

-- Empty-body rows get an empty paragraph doc.
UPDATE comments
SET body = '{"type":"doc","content":[{"type":"paragraph","content":[]}]}'
WHERE body IS NULL OR body = '';

-- =====================
-- Step 2: ALTER text → jsonb.
-- =====================
ALTER TABLE comments ALTER COLUMN body TYPE jsonb USING body::jsonb;

-- Ensure the default is a valid empty TipTap doc for future inserts.
ALTER TABLE comments ALTER COLUMN body SET DEFAULT
    '{"type":"doc","content":[{"type":"paragraph","content":[]}]}'::jsonb;

COMMIT;
