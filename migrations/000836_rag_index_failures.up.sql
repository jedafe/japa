-- 000836_rag_index_failures
-- §69 admin visibility for fire-and-forget RAG indexing: when the shared
-- retry helper exhausts all attempts for an index/purge call, the call site
-- records the entity here so GET /admin/rag/status can surface it and
-- POST /admin/rag/retry-failures can re-enqueue it.
BEGIN;

CREATE TABLE IF NOT EXISTS rag_index_failures (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type  text NOT NULL,
    entity_id    uuid NOT NULL,
    operation    text NOT NULL CHECK (operation IN ('index', 'purge')),
    error        text NOT NULL,
    attempted_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (entity_type, entity_id, operation)
);

CREATE INDEX IF NOT EXISTS idx_rag_index_failures_attempted
    ON rag_index_failures (attempted_at DESC);

COMMIT;
