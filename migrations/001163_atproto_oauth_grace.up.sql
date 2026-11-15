-- A6-5 review fixes (§127):
--   request_uri        — links a grant to the PAR that minted it so a
--                        code-replay revoke hits exactly that grant, not
--                        the whole client×account family.
--   refresh_rotated_at — stamped at every rotation; a prev_refresh_hash
--                        replayed within 30s is a benign concurrent
--                        retry (invalid_grant, no revoke), an older one
--                        is theft (family revoke). §120 grace posture.
ALTER TABLE atproto_oauth_tokens
    ADD COLUMN request_uri text,
    ADD COLUMN refresh_rotated_at timestamptz;

CREATE INDEX idx_atproto_oauth_tokens_request_uri
    ON atproto_oauth_tokens(request_uri) WHERE request_uri IS NOT NULL;
