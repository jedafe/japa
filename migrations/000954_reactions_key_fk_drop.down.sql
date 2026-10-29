-- Restore the FK — first purge reaction rows whose key no longer resolves
-- to a reaction_types row (e.g. emoji-keyed achievement reactions).
DELETE FROM public.reactions r
WHERE NOT EXISTS (
    SELECT 1 FROM public.reaction_types rt WHERE rt.key = r.reaction_key
);

ALTER TABLE ONLY public.reactions
    ADD CONSTRAINT reactions_reaction_key_fkey FOREIGN KEY (reaction_key) REFERENCES public.reaction_types(key) ON DELETE CASCADE;
