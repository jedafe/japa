-- 000718_intel_inbound_retention.up.sql
-- Seed the intel-channels inbound-event retention setting consumed by the
-- daily intel:purge_inbound_events sweep (audit follow-up — raw delivery
-- payloads in intel_inbound_events carry sender PII: addresses, subjects,
-- bodies; they were previously kept forever).
--
--   intel.inbound.retention_days — days an intel_inbound_events row is kept
--                                  (measured from received_at) before the
--                                  sweep deletes it. 30 covers the admin
--                                  redrive window for failed/stuck
--                                  deliveries while bounding PII retention.
--                                  0 = keep forever (sweep no-ops).

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    ('intel.inbound.retention_days', to_jsonb(30))
ON CONFLICT (key) DO NOTHING;

COMMIT;
