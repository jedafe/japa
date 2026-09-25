-- 000384_comments_body_text_to_jsonb.down.sql
-- Reverses 000384: converts comments.body back from jsonb to text.
-- Extracts plain text from the TipTap doc for backward compatibility.

BEGIN;

-- Extract the first text node's text as the plain-text body.
-- Falls back to the raw JSON string if extraction fails.
ALTER TABLE comments ALTER COLUMN body TYPE text USING
    COALESCE(
        body->'content'->0->'content'->0->>'text',
        body::text
    );

ALTER TABLE comments ALTER COLUMN body DROP DEFAULT;

COMMIT;
