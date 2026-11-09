-- 001095 — content thread anchors (unified-discussion spec): every
-- published, public contents row gets its own canonical `discussion`
-- thread row. The article is no longer its own thread — the thread is a
-- real discussion row (own short body, own comments, own vote/bookmark
-- state) bound two ways:
--
--   thread_anchors(entity_type='content', entity_key=<article uuid>)  →
--       canonical get-or-create handle; /thread/{slug} and the entity
--       page's discussion preview resolve through it.
--   content_thread_links(content_id=<article>, thread_id=<thread>)    →
--       §49 parent link; GetThreadParentByThreadID renders the article
--       embed card and comment counts propagate up to the article.
--
-- Live minting runs in-process on the publish seams (createContent,
-- UpdateContentStatus, PATCH-driven publish) and lazily via
-- POST /threads/anchor; this backfill covers everything already live.
-- discussion/forum_thread rows are skipped — they ARE threads.

-- 0. Widen the entity_type domain — 'content' anchors join 'tool' and
--    'wiki_page'.
ALTER TABLE public.thread_anchors
    DROP CONSTRAINT thread_anchors_entity_type_check,
    ADD CONSTRAINT thread_anchors_entity_type_check
        CHECK (entity_type = ANY (ARRAY['tool', 'wiki_page', 'content']));

-- 1. Mint the discussion rows. Slug is deterministic ('thread-' + parent
--    uuid hex) so the RETURNING join is exact and re-runs can't collide.
--    The thread files under the parent's primary space (content_spaces
--    is_primary, falling back to contents.space_id) — a private/inactive
--    space means no public thread, same leak guard as wiki pages.
WITH eligible AS (
    SELECT c.id, c.author_id, c.title, c.slug, c.content_type, c.visibility,
           c.geo_space_id, c.meta_description,
           COALESCE(ps.space_id, c.space_id) AS primary_space_id
    FROM contents c
    LEFT JOIN LATERAL (
        SELECT cs.space_id FROM content_spaces cs
        WHERE cs.content_id = c.id AND cs.is_primary
        LIMIT 1
    ) ps ON true
    JOIN spaces s ON s.id = COALESCE(ps.space_id, c.space_id)
        AND s.deleted_at IS NULL
        AND s.visibility = 'public' AND s.status = 'active'
    WHERE c.deleted_at IS NULL
      AND c.status = 'published'
      AND c.visibility = 'public'
      AND c.content_type NOT IN ('discussion', 'forum_thread')
      AND NOT EXISTS (
          SELECT 1 FROM thread_anchors ta
          WHERE ta.entity_type = 'content' AND ta.entity_key = c.id::text
      )
),
new_threads AS (
    INSERT INTO public.contents (
        author_id, space_id, geo_space_id, content_type, title, slug,
        body, body_html, status, visibility, published_at
    )
    SELECT
        e.author_id,
        e.primary_space_id,
        e.geo_space_id,
        'discussion',
        'Discussing: ' || e.title,
        'thread-' || replace(e.id::text, '-', ''),
        -- Same body the in-process minter writes: the article summary
        -- plus a conversation prompt (TipTap doc, raw JSON in `body`).
        jsonb_build_object('type', 'doc', 'content', jsonb_build_array(
            jsonb_build_object('type', 'paragraph', 'content', jsonb_build_array(
                jsonb_build_object('type', 'text', 'text',
                    COALESCE(NULLIF(btrim(e.meta_description), ''), 'Discussing: ' || e.title)))),
            jsonb_build_object('type', 'paragraph', 'content', jsonb_build_array(
                jsonb_build_object('type', 'text', 'text',
                    'Questions, experiences, and tips about ' || e.title || ' — join the discussion below.')))
        ))::text,
        '',
        'published',
        e.visibility,
        now()
    FROM eligible e
    ON CONFLICT DO NOTHING
    RETURNING id, slug
),
linked AS (
    INSERT INTO public.content_thread_links (content_id, thread_id)
    SELECT e.id, nt.id
    FROM eligible e
    JOIN new_threads nt ON nt.slug = 'thread-' || replace(e.id::text, '-', '')
    ON CONFLICT (content_id, thread_id) DO NOTHING
    RETURNING thread_id
)
INSERT INTO public.thread_anchors (content_id, entity_type, entity_key, entity_url, entity_title, entity_summary)
SELECT
    nt.id,
    'content',
    e.id::text,
    -- Canonical detail route per content_type (mirrors contentLink.ts).
    CASE e.content_type
        WHEN 'blog_post'       THEN '/blog/' || e.slug
        WHEN 'guide'           THEN '/guide/' || e.slug
        WHEN 'comparison'      THEN '/comparisons/' || e.slug
        WHEN 'story'           THEN '/story/' || e.slug
        WHEN 'achievement'     THEN '/achievements/' || e.slug
        WHEN 'page'            THEN '/pages/' || e.slug
        WHEN 'job_listing'     THEN '/jobs/' || e.slug
        WHEN 'service_listing' THEN '/services/' || e.slug
        WHEN 'video'           THEN '/video/' || e.slug
        WHEN 'list'            THEN '/list/' || e.slug
        ELSE '/content/' || e.slug
    END,
    e.title,
    NULLIF(btrim(e.meta_description), '')
FROM eligible e
JOIN new_threads nt ON nt.slug = 'thread-' || replace(e.id::text, '-', '')
ON CONFLICT (entity_type, entity_key) DO NOTHING;

-- 2. Primary-space junction for each minted thread — mirrors the parent's
--    primary space so space listings and perms resolve (BUG-008 shape).
INSERT INTO public.content_spaces (content_id, space_id, is_primary)
SELECT ta.content_id, cs.space_id, true
FROM thread_anchors ta
JOIN contents e ON e.id::text = ta.entity_key
JOIN content_spaces cs ON cs.content_id = e.id AND cs.is_primary
WHERE ta.entity_type = 'content'
  AND NOT EXISTS (
      SELECT 1 FROM content_spaces x
      WHERE x.content_id = ta.content_id AND x.is_primary
  );

-- 3. Move the comments that lived on the article row onto its canonical
--    thread — the article's discussion now lives on the discussion row.
UPDATE public.comments cm
SET content_id = ta.content_id
FROM thread_anchors ta
WHERE ta.entity_type = 'content'
  AND cm.content_id::text = ta.entity_key;

-- 4. Recount comment_count everywhere: each row's own live comments plus
--    comments on its linked threads (the propagation the write path now
--    maintains via IncrementLinkedContentCommentCount).
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

-- 5. Repair anchor rows minted before body_html was supplied: createContent
--    had run the TipTap JSON through the markdown renderer, storing the doc
--    as one literal-JSON paragraph. Resetting to '' makes the renderer fall
--    back to decoding the raw TipTap `body`.
UPDATE public.contents
SET body_html = ''
WHERE content_type = 'discussion'
  AND body LIKE '{%'
  AND body_html LIKE '%&#34;%';
