-- Roll back the forum thread contents backfill. Rows created by the up
-- migration are identified by content_type='forum_thread' AND
-- content_hash IS NULL (handler-created contents always carry a hash).
UPDATE forum_threads ft
SET content_id = NULL
WHERE content_id IN (
    SELECT id FROM contents
    WHERE content_type = 'forum_thread' AND content_hash IS NULL
);

DELETE FROM contents
WHERE content_type = 'forum_thread' AND content_hash IS NULL;
