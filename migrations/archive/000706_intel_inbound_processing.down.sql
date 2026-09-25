-- Revert the 'processing' status: stranded 'processing' rows are reset to
-- 'received' before the constraint narrows so the CHECK still passes.
ALTER TABLE intel_inbound_events DROP COLUMN IF EXISTS claimed_at;
UPDATE intel_inbound_events SET status = 'received', processed_at = NULL
WHERE status = 'processing';
ALTER TABLE intel_inbound_events DROP CONSTRAINT IF EXISTS intel_inbound_events_status_check;
ALTER TABLE intel_inbound_events ADD CONSTRAINT intel_inbound_events_status_check
  CHECK (status IN ('received','processed','failed','ignored'));
