-- Phase 16: Search analytics for zero-result detection and trending queries.
-- Migration 000104: creates search_analytics table.

CREATE TABLE IF NOT EXISTS search_analytics (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id     uuid NOT NULL REFERENCES tenants(id),
    query         text NOT NULL,
    results_count int NOT NULL DEFAULT 0,
    user_id       uuid REFERENCES users(id),
    content_type  text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_search_analytics_query ON search_analytics (tenant_id, query, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_search_analytics_zero ON search_analytics (tenant_id, results_count, created_at DESC)
    WHERE results_count = 0;
