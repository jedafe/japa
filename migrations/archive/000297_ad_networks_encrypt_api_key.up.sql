-- Rename api_key → api_key_encrypted so the column name reflects its
-- encrypted-at-rest contents (AES-256-GCM via internal/crypto, same
-- pattern as ai_providers.api_key_encrypted).
-- Existing plaintext values are preserved as-is; crypto.Decrypt handles
-- legacy plaintext transparently (returns as-is with a warning log).
-- On the next Create/Update the service layer re-encrypts properly.
ALTER TABLE ad_networks RENAME COLUMN api_key TO api_key_encrypted;
