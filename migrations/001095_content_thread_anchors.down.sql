-- 001095 down — unwind content thread anchors: move the comments back onto
-- the article rows, then drop the minted discussion rows, links, anchors,
-- and junction rows. Only rows bound by entity_type='content' anchors are
-- touched — tool/wiki anchors and §49 user-started linked discussions stay.

-- 1. Comments return to the article that owned them pre-migration.
UPDATE public.comments cm
SET content_id = ta.entity_key::uuid
FROM thread_anchors ta
WHERE ta.entity_type = 'content'
  AND cm.content_id = ta.content_id;

-- 2. §49 links recorded for content-anchored threads.
DELETE FROM public.content_thread_links l
USING thread_anchors ta
WHERE ta.entity_type = 'content'
  AND l.thread_id = ta.content_id;

-- 3. Junction + anchor rows for the minted threads.
DELETE FROM public.content_spaces cs
USING thread_anchors ta
WHERE ta.entity_type = 'content'
  AND cs.content_id = ta.content_id;

DELETE FROM public.contents
WHERE id IN (
    SELECT content_id FROM thread_anchors WHERE entity_type = 'content'
);

DELETE FROM public.thread_anchors WHERE entity_type = 'content';

-- 4. Recount comment_count after the move-back (own live comments + any
--    remaining linked-thread comments).
UPDATE public.contents c
SET comment_count = COALESCE(own.cnt, 0) + COALESCE(lnk.cnt, 0)
FROM contents c2
LEFT JOIN (
    SELECT content_id, COUNT(*)::int AS cnt
    FROM comments WHERE deleted_at IS NULL
    GROUP BY content_id
) own ON own.content_id = c2.id
LEFT JOIN (
    SELECT l.content_id, COUNT(cm.id)::int AS cnt
    FROM content_thread_links l
    JOIN comments cm ON cm.content_id = l.thread_id AND cm.deleted_at IS NULL
    GROUP BY l.content_id
) lnk ON lnk.content_id = c2.id
WHERE c.id = c2.id;

-- 5. Restore the entity_type domain.
ALTER TABLE public.thread_anchors
    DROP CONSTRAINT thread_anchors_entity_type_check,
    ADD CONSTRAINT thread_anchors_entity_type_check
        CHECK (entity_type = ANY (ARRAY['tool', 'wiki_page']));
