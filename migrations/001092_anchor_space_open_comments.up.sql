-- 001092 — open commenting/replying on the thread-anchor space.
--
-- 001091 seeded community-threads with commenting_perm/replying_perm =
-- 'members', which gates every tool/wiki anchor thread behind membership
-- in a hidden platform space — a user who never joined could not comment.
-- Anchor threads are entity discussion surfaces, not space content, so the
-- write axes relax to 'open' (any authenticated member). posting_perm and
-- joining_perm stay as seeded: posts are system-minted via the anchor
-- endpoint (create path bypasses the space check), and joining still works
-- for anyone who finds the space page.

BEGIN;

UPDATE public.spaces
SET commenting_perm = 'open',
    replying_perm = 'open'
WHERE slug = 'community-threads';

COMMIT;
