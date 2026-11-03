-- One reaction per user per entity (product rule: a user picks a single
-- reaction on a post — selecting another key switches it instead of
-- stacking). Dedupe existing rows, keeping the most recently created
-- reaction per (entity_type, entity_id, user_id), then tighten the primary
-- key so the invariant is enforced at write time.
DELETE FROM reactions a
USING reactions b
WHERE a.entity_type = b.entity_type
  AND a.entity_id   = b.entity_id
  AND a.user_id     = b.user_id
  AND (a.created_at < b.created_at
       OR (a.created_at = b.created_at AND a.reaction_key < b.reaction_key));

ALTER TABLE reactions DROP CONSTRAINT reactions_pkey;
ALTER TABLE reactions
    ADD CONSTRAINT reactions_pkey PRIMARY KEY (entity_type, entity_id, user_id);
