-- 000729_content_embeddings_scope.up.sql — Scoped retrieval (§69 amendment).
-- Adds denormalized scope metadata (visibility, space_id, author_id) to
-- content_embeddings so retrieval can filter by the caller's view without
-- joins on the vector hot path. Backfills existing rows from source
-- entities; unresolvable/orphaned rows fail closed to 'private'.

ALTER TABLE content_embeddings
    ADD COLUMN visibility text NOT NULL DEFAULT 'public',
    ADD COLUMN space_id   uuid NULL,
    ADD COLUMN author_id  uuid NULL;

ALTER TABLE content_embeddings
    ADD CONSTRAINT content_embeddings_visibility_chk
    CHECK (visibility IN ('public','members','private'));

UPDATE content_embeddings ce
SET    visibility = s.visibility, space_id = s.id
FROM   spaces s
WHERE  ce.entity_type = 'space' AND ce.entity_id = s.id;

UPDATE content_embeddings ce
SET    visibility = c.visibility, space_id = c.space_id, author_id = c.author_id
FROM   contents c
WHERE  ce.entity_type = 'content' AND ce.entity_id = c.id;

UPDATE content_embeddings ce
SET    visibility = sp.visibility, space_id = ft.space_id,
       author_id = ft.author_user_id
FROM   forum_threads ft
JOIN   spaces sp ON sp.id = ft.space_id
WHERE  ce.entity_type = 'forum_thread' AND ce.entity_id = ft.id;

-- Threads whose parent space is gone (space_id SET NULL on space delete)
-- and any entity rows that failed to resolve above fail closed.
UPDATE content_embeddings SET visibility = 'private'
WHERE  entity_type = 'forum_thread' AND space_id IS NULL;

UPDATE content_embeddings SET visibility = 'private'
WHERE  entity_type = 'content'
  AND NOT EXISTS (SELECT 1 FROM contents c WHERE c.id = content_embeddings.entity_id);

UPDATE content_embeddings SET visibility = 'private'
WHERE  entity_type = 'space'
  AND NOT EXISTS (SELECT 1 FROM spaces s WHERE s.id = content_embeddings.entity_id);

-- wiki_page / learning_track remain 'public' (public corpora by design).

ALTER TABLE content_embeddings ALTER COLUMN visibility DROP DEFAULT;

CREATE INDEX idx_content_embeddings_scope
    ON content_embeddings (visibility, space_id);
