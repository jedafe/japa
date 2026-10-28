-- Follows audit BUG-012: align the follows.entity_type CHECK constraint with
-- the shipped entity set (user|space|tag|organization|source).
-- 'professional_profile' was allowed by the baseline schema but is rejected
-- by the service layer (isValidEntityType), leaving a permissive schema
-- value with no valid writer. Drop it and purge any dead rows first.

-- Remove rows with entity types no longer shipped (dead data; nothing can
-- create them through the service today).
DELETE FROM follows
WHERE entity_type NOT IN ('user', 'space', 'tag', 'organization', 'source');

ALTER TABLE public.follows DROP CONSTRAINT follows_entity_type_check;

ALTER TABLE public.follows
    ADD CONSTRAINT follows_entity_type_check
    CHECK ((entity_type = ANY (ARRAY['user'::text, 'space'::text, 'tag'::text, 'organization'::text, 'source'::text])));
