-- Backfill backing contents rows for forum threads created before
-- content_id existed (pre-m000636). Without a contents row the thread is
-- unreachable by search, indexing, bookmarks, reports, viewer-state and
-- tag attachment — every /content/{id} route 404s for it.
--
-- content_hash is intentionally NULL: Rule 12's hash is computed
-- application-side (strip_markdown + sha256) and cannot be reproduced on
-- TipTap JSON bodies in pure SQL. NULL also serves as a safe marker —
-- handler-created contents rows always carry a hash, so the down
-- migration can delete exactly the rows this migration created.
DO $$
DECLARE
    t        RECORD;
    new_cid  uuid;
    new_slug text;
BEGIN
    FOR t IN
        SELECT id, space_id, author_user_id, title, slug, body, created_at
        FROM forum_threads
        WHERE content_id IS NULL
          AND deleted_at IS NULL
          AND author_user_id IS NOT NULL
    LOOP
        -- contents.slug is globally unique (idx_contents_slug): dedupe
        -- deterministically with a thread-id fragment.
        new_slug := t.slug;
        IF EXISTS (SELECT 1 FROM contents WHERE slug = new_slug) THEN
            new_slug := t.slug || '-' || substr(t.id::text, 1, 8);
        END IF;

        INSERT INTO contents (author_id, space_id, content_type, title, slug,
                              body, body_html, field_values, content_hash,
                              seo_indexable, status, visibility,
                              read_time_minutes, published_at)
        VALUES (t.author_user_id, t.space_id, 'forum_thread', t.title, new_slug,
                t.body::text, '', '{}'::jsonb, NULL,
                true, 'published', 'public',
                0, t.created_at)
        RETURNING id INTO new_cid;

        UPDATE forum_threads SET content_id = new_cid WHERE id = t.id;
    END LOOP;
END $$;
