-- Atomic backstop for the api_key_hash dedup check: the SELECT-then-INSERT
-- in UserAIKeyStore.Create/Update is racy under concurrent requests; the
-- partial unique index makes cross-account credential sharing impossible
-- at the constraint layer. Rows predating hashing ('' hashes) are exempt.
CREATE UNIQUE INDEX ux_user_ai_keys_key_hash ON user_ai_keys(api_key_hash) WHERE api_key_hash <> '';
