-- E-001 fix: reactions.reaction_key is polymorphic-domain — the curated
-- achievement set (§89) uses emoji as keys (e.g. '🎉'), which can never
-- exist in reaction_types.key. The FK therefore made every achievement
-- reaction insert fail with 23503. Key validity is now enforced in
-- application code (enabled-type check in AddReaction); deleting a
-- reaction type deletes its orphan reaction rows via DeleteReactionsByKey.
ALTER TABLE ONLY public.reactions
    DROP CONSTRAINT reactions_reaction_key_fkey;
