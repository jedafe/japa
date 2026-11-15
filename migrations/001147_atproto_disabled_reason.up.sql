-- 001147_atproto_disabled_reason.up.sql
-- atproto_accounts.disabled_reason records WHO froze the account:
-- 'member'    — member-initiated disable (DELETE /me/atproto); only the
--               member's own opt-in may reactivate.
-- 'lifecycle' — platform-status freeze driven by atproto:lifecycle_sync
--               (suspend/ban/restrict/delete); auto-reactivates when the
--               user returns to 'active'.
-- NULL reads as member-initiated: every disable that predates this
-- column came through the member endpoint. Cleared on reactivation and
-- irrelevant for 'active'/'tombstone' rows.
--
-- Ref: japaconnect-engineering/sections/127-atproto-native.md (Lifecycle)

BEGIN;

ALTER TABLE atproto_accounts
    ADD COLUMN IF NOT EXISTS disabled_reason text;

ALTER TABLE atproto_accounts
    ADD CONSTRAINT atproto_accounts_disabled_reason_check
    CHECK (disabled_reason IS NULL OR disabled_reason IN ('member', 'lifecycle'));

COMMIT;
