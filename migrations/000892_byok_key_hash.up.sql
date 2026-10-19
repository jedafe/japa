-- api_key_hash lets us detect the same credential attached to multiple
-- accounts (a strong stolen-key laundering signal) without storing the key
-- in the clear alongside it. sha256 of the trimmed plaintext — high-entropy
-- input, no salt needed.
ALTER TABLE user_ai_keys ADD COLUMN api_key_hash TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_user_ai_keys_key_hash ON user_ai_keys(api_key_hash);
