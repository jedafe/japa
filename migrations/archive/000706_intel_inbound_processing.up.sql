-- intel-channels Phase 3 hardening: add 'processing' to the inbound event
-- status enum so the intel:inbound_email task can atomically claim an
-- event (received → processing) before doing work — two workers can no
-- longer both read 'received' and double-materialize the delivery.
-- claimed_at stamps the claim so the admin re-drive can distinguish a
-- live claim from a stuck one (only processing events whose claim is
-- older than the task-timeout+slack window may be reset).
ALTER TABLE intel_inbound_events DROP CONSTRAINT IF EXISTS intel_inbound_events_status_check;
ALTER TABLE intel_inbound_events ADD CONSTRAINT intel_inbound_events_status_check
  CHECK (status IN ('received','processing','processed','failed','ignored'));
ALTER TABLE intel_inbound_events ADD COLUMN IF NOT EXISTS claimed_at timestamptz;
