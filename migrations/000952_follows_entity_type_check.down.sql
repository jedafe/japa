-- Restore the permissive CHECK including 'professional_profile'.
ALTER TABLE public.follows DROP CONSTRAINT follows_entity_type_check;

ALTER TABLE public.follows
    ADD CONSTRAINT follows_entity_type_check
    CHECK ((entity_type = ANY (ARRAY['user'::text, 'space'::text, 'tag'::text, 'organization'::text, 'professional_profile'::text, 'source'::text])));
