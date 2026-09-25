-- Down migration for 000636: unlink backfilled contents rows and soft-delete
-- them rather than hard-deleting user content (Rule 11).

UPDATE forum_threads ft
SET content_id = NULL
FROM contents c
WHERE c.content_type = 'forum_thread'
  AND c.field_values -> 'backfilled_by_m000636' = 'true'
  AND ft.content_id = c.id;

UPDATE contents
SET deleted_at = now()
WHERE content_type = 'forum_thread'
  AND field_values -> 'backfilled_by_m000636' = 'true';
