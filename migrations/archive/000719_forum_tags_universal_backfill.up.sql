-- Backfill forum-specific tags onto the universal tags system (Sections 08, 14).
-- Forum threads attach tags through their backing contents row
-- (forum_threads.content_id → content_tags). After this migration the
-- forum code reads/writes `tags` and `content_tags` only; the legacy
-- forum_tags / forum_thread_tags tables are retained for rollback.

-- 1) Copy forum_tags rows into tags, preserving ids where the slug is free.
--    Slug collisions (same slug already exists in tags) are skipped here —
--    step 2 remaps their thread-tag rows to the existing universal tag.
INSERT INTO tags (id, name, slug, description, rules, cover_media_id, follower_count, is_system, created_at, updated_at)
SELECT ft.id, ft.name, ft.slug, ft.description, ft.rules, ft.cover_image_id,
       ft.follower_count, false, ft.created_at, now()
FROM forum_tags ft
WHERE ft.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM tags t WHERE t.slug = ft.slug)
ON CONFLICT (slug) DO NOTHING;

-- 2) Copy thread-tag attachments into content_tags via the thread's backing
--    content row. When the forum tag's slug already existed in tags under a
--    different id, the attachment is remapped to the existing universal tag.
INSERT INTO content_tags (content_id, tag_id)
SELECT ft.content_id,
       COALESCE(existing.id, ftt.tag_id)
FROM forum_thread_tags ftt
JOIN forum_threads ft ON ft.id = ftt.thread_id
JOIN forum_tags ftag ON ftag.id = ftt.tag_id
LEFT JOIN tags existing ON existing.slug = ftag.slug AND existing.id <> ftt.tag_id
WHERE ft.content_id IS NOT NULL
  AND ftag.deleted_at IS NULL
ON CONFLICT (content_id, tag_id) DO NOTHING;

-- 3) Copy forum tag moderator assignments into tag_moderators.
INSERT INTO tag_moderators (tag_id, user_id, assigned_by, assigned_by_user_id)
SELECT COALESCE(existing.id, ft.id), ft.moderator_user_id, 'site_admin', ft.moderator_user_id
FROM forum_tags ft
LEFT JOIN tags existing ON existing.slug = ft.slug AND existing.id <> ft.id
WHERE ft.moderator_user_id IS NOT NULL
  AND ft.deleted_at IS NULL
ON CONFLICT (tag_id, user_id) DO NOTHING;
