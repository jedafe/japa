ALTER TABLE intel_source_channels
    ADD COLUMN disabled_by text
    CONSTRAINT intel_source_channels_disabled_by_check
    CHECK (disabled_by IN ('circuit_breaker', 'license', 'admin'));

-- Backfill: rows the failure circuit-breakers already disabled carry
-- consecutive_failures > 0 + a stamped error; attribute them so the
-- re-probe sweep can pick them up. The rest are operator disables.
UPDATE intel_source_channels SET disabled_by = 'circuit_breaker'
WHERE enabled = false
  AND consecutive_failures > 0
  AND last_error_message IS NOT NULL;

UPDATE intel_source_channels SET disabled_by = 'admin'
WHERE enabled = false
  AND disabled_by IS NULL;
