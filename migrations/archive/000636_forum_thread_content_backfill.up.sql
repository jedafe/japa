-- Section 08 / Section 14: backfill backing contents rows for legacy forum
-- threads that were created before the content-row requirement, and recompute
-- denormalized reply counts to match the actual number of non-deleted replies
-- (including nested replies).

-- Recompute reply_count so it equals the flat count of non-deleted replies.
-- This fixes the mismatch between the thread card count and the reply list heading.
UPDATE forum_threads ft
SET reply_count = (
    SELECT COUNT(*)::int
    FROM forum_replies fr
    WHERE fr.thread_id = ft.id AND fr.deleted_at IS NULL
)
WHERE ft.deleted_at IS NULL;

-- Create a backing contents row for each forum thread that does not have one.
-- This enables reporting, related content, and other content-system features
-- (Section 14). content_hash is intentionally NULL for the backfill; the unique
-- index allows NULL hashes and the application re-computes it on the next edit.
INSERT INTO contents (
    tenant_id,
    author_id,
    space_id,
    content_type,
    title,
    slug,
    body,
    body_html,
    field_values,
    status,
    visibility,
    is_pinned,
    view_count,
    comment_count,
    published_at,
    created_at,
    updated_at
)
SELECT
    ft.tenant_id,
    ft.author_user_id,
    ft.space_id,
    'forum_thread',
    ft.title,
    ft.slug,
    COALESCE(NULLIF(TRIM(BOTH '"' FROM ft.body::text), ''), ''),
    '',
    jsonb_build_object('backfilled_by_m000636', true),
    'published',
    'public',
    false,
    ft.view_count,
    ft.reply_count,
    ft.created_at,
    ft.created_at,
    ft.updated_at
FROM forum_threads ft
WHERE ft.content_id IS NULL
  AND ft.deleted_at IS NULL
ON CONFLICT (tenant_id, slug) WHERE deleted_at IS NULL DO NOTHING;

-- Link the newly created contents rows back to the forum threads.
UPDATE forum_threads ft
SET content_id = c.id
FROM contents c
WHERE c.content_type = 'forum_thread'
  AND c.field_values -> 'backfilled_by_m000636' = 'true'
  AND ft.content_id IS NULL
  AND ft.tenant_id = c.tenant_id
  AND ft.slug = c.slug
  AND ft.deleted_at IS NULL;
