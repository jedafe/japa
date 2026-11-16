-- 001176: relay unwinds look subscriptions up by delivery-target inbox —
-- SetFederationRelayStatusByInbox's WHERE inbox_url = ... probe would
-- seq-scan on every dead-letter otherwise. Matches the §126 DDL.
BEGIN;

CREATE INDEX IF NOT EXISTS idx_federation_relays_inbox ON federation_relays (inbox_url);

COMMIT;
