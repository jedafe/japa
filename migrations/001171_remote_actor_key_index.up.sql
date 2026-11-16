-- §126 M2: every inbound signed POST resolves the signer through
-- GetRemoteActorByKeyID (public_key_id + status='active'). Without an
-- index the lookup seq-scans remote_actors once per delivery — the
-- hottest read in the inbound pipeline.
CREATE INDEX IF NOT EXISTS idx_remote_actors_key_id ON remote_actors (public_key_id);
