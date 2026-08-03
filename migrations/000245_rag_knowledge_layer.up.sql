-- 000245_rag_knowledge_layer.up.sql
-- JapaBot RAG Knowledge Layer (Section 69, ADR-069).
-- Creates the content_embeddings table with pgvector HNSW index for
-- retrieval-augmented generation over JapaConnect's published content corpus.
-- Also seeds the 4 rag.* system_settings and 2 ai.rag.* RBAC permission nodes.
--
-- Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
--      japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md

BEGIN;

-- =====================================================================
-- content_embeddings — one row per content chunk (not per document)
-- =====================================================================
CREATE TABLE IF NOT EXISTS content_embeddings (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id),
    entity_type  text NOT NULL,   -- 'content'|'forum_thread'|'space'|'learning_track'
    entity_id    uuid NOT NULL,
    chunk_index  int  NOT NULL DEFAULT 0,
    chunk_text   text NOT NULL,
    embedding    vector(1536) NOT NULL,
    language     text NOT NULL DEFAULT 'en',
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, entity_type, entity_id, chunk_index)
);

-- HNSW index for approximate nearest-neighbour cosine similarity search.
CREATE INDEX IF NOT EXISTS idx_content_embeddings_hnsw
    ON content_embeddings USING hnsw (embedding vector_cosine_ops);

-- B-tree index for entity-scoped lookups and deletes.
CREATE INDEX IF NOT EXISTS idx_content_embeddings_entity
    ON content_embeddings (tenant_id, entity_type, entity_id);

-- =====================================================================
-- system_settings — 4 rag.* keys (Section 69, ADR-069)
-- Seeded for the default tenant only; migration 000240 backfills copies
-- to other tenants.
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'rag.enabled',             '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'rag.embedding_model',     '"text-embedding-3-small"'),
    ('00000000-0000-0000-0000-000000000001', 'rag.retrieval_top_k',     '"5"'),
    ('00000000-0000-0000-0000-000000000001', 'rag.chunk_max_tokens',    '"400"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 2 ai.rag.* nodes (Section 69, ADR-069)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('ai.rag.manage',  'Configure RAG settings (admin only)'),
    ('ai.rag.reindex', 'Trigger full corpus re-index (admin only)')
ON CONFLICT (key) DO NOTHING;

-- Grant both ai.rag.* nodes to super_admin (admin capabilities, not
-- ai.context.* user-feed nodes, so they ARE granted to super_admin).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('ai.rag.manage', 'ai.rag.reindex')
ON CONFLICT DO NOTHING;

COMMIT;
