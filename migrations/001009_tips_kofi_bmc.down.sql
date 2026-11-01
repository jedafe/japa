ALTER TABLE tips DROP CONSTRAINT tips_method_check;
ALTER TABLE tips ADD CONSTRAINT tips_method_check
    CHECK (method = ANY (ARRAY['points','credits','stripe','paystack','nowpayments','manual_transfer','opencollective']));
ALTER TABLE tip_settings
    DROP COLUMN IF EXISTS kofi_webhook_secret,
    DROP COLUMN IF EXISTS bmc_page_slug,
    DROP COLUMN IF EXISTS bmc_api_token_encrypted;
