-- §127 A6-2 did:plc: rotation-key custody + old-DID provenance.
-- plc_rotation_key_enc holds a second encrypted K-256 scalar (the
-- did-method-plc rotation key) — plc_operations are signed by it, never
-- by the repo signing key. NULL until the account's first provisioning
-- attempt. previous_did stores the pre-migration did:web so old-DID
-- provenance (embedded at:// refs, late remote replies/follows, the
-- did:web doc URL) stays resolvable after the swap.
ALTER TABLE atproto_accounts
  ADD COLUMN plc_rotation_key_enc text,
  ADD COLUMN previous_did text;

-- previous_did lookups are on the fallback path only (URI/DID misses) —
-- a partial index keeps them cheap without taxing the common case.
CREATE UNIQUE INDEX atproto_accounts_previous_did_key
  ON atproto_accounts (previous_did) WHERE previous_did IS NOT NULL;
