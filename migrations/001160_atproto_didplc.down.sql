DROP INDEX IF EXISTS atproto_accounts_previous_did_key;
ALTER TABLE atproto_accounts
  DROP COLUMN plc_rotation_key_enc,
  DROP COLUMN previous_did;
