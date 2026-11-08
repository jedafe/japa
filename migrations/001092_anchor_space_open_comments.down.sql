BEGIN;

UPDATE public.spaces
SET commenting_perm = 'members',
    replying_perm = 'members'
WHERE slug = 'community-threads';

COMMIT;
