-- §125 Ko-fi + Buy Me a Coffee external donation sources.
-- tips.method widened; tip_settings gains per-source credentials.
-- oc_contribution_id is reused for all external dedupe ids, namespaced
-- ('kofi:<message_id>', 'bmc:<support_id>') so the UNIQUE index stays uniform.
ALTER TABLE tip_settings
    ADD COLUMN kofi_webhook_secret      text,
    ADD COLUMN bmc_page_slug            text,
    ADD COLUMN bmc_api_token_encrypted  text;

ALTER TABLE tips DROP CONSTRAINT tips_method_check;
ALTER TABLE tips ADD CONSTRAINT tips_method_check
    CHECK (method = ANY (ARRAY['points','credits','stripe','paystack','nowpayments','manual_transfer','opencollective','kofi','buymeacoffee']));
