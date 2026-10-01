-- Soft-delete the duplicate Nairobi state space. ISO 3166-2:KE-30 is Nairobi
-- (officially "Nairobi City"); the ISO seed created 'ke-nairobi-city' while
-- the baseline seed added 'nairobi-county' for the same county. The county
-- space is kept — it owns the 'nairobi' city child; 'ke-nairobi-city' has no
-- members, contents, wiki pages, or children. Its junction edge is retained,
-- consistent with other soft-deleted spaces.
UPDATE public.spaces
SET status = 'deleted', deleted_at = now()
WHERE slug = 'ke-nairobi-city' AND deleted_at IS NULL;
