-- Index for one-time activity rule enforcement and wallet source lookups.
-- Supports CountUserLedgerBySource (Section 28).
CREATE INDEX IF NOT EXISTS idx_wallet_ledger_tenant_user_source ON wallet_ledger (tenant_id, user_id, source);
