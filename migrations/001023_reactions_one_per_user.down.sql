-- Restore the per-(user, key) primary key. Note: rows deleted by the up
-- migration's dedupe step cannot be restored.
ALTER TABLE reactions DROP CONSTRAINT reactions_pkey;
ALTER TABLE reactions
    ADD CONSTRAINT reactions_pkey PRIMARY KEY (entity_type, entity_id, user_id, reaction_key);
