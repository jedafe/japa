-- Roll back the forum-tags → universal-tags backfill (000719).
-- Removes only the rows this migration created: content_tags attachments
-- sourced from forum_thread_tags, tag_moderator rows sourced from
-- forum_tags.moderator_user_id, and tags rows copied from forum_tags that
-- still have no content attached.

DELETE FROM tag_moderators tm
USING forum_tags ft
WHERE tm.user_id = ft.moderator_user_id
  AND ft.moderator_user_id IS NOT NULL
  AND tm.tag_id IN (
      SELECT COALESCE(existing.id, ft.id)
      FROM forum_tags ft2
      LEFT JOIN tags existing ON existing.slug = ft2.slug AND existing.id <> ft2.id
      WHERE ft2.id = ft.id
  );

DELETE FROM content_tags ct
USING forum_threads ft, forum_thread_tags ftt, forum_tags ftag
WHERE ct.content_id = ft.content_id
  AND ftt.thread_id = ft.id
  AND ftag.id = ftt.tag_id
  AND ct.tag_id = COALESCE(
      (SELECT t.id FROM tags t WHERE t.slug = ftag.slug AND t.id <> ftt.tag_id),
      ftt.tag_id
  );

-- Only remove tags that were copied from forum_tags and remain unused;
-- a tag that has since been attached to non-forum content is kept.
DELETE FROM tags t
USING forum_tags ft
WHERE t.id = ft.id
  AND NOT EXISTS (SELECT 1 FROM content_tags ct WHERE ct.tag_id = t.id);
