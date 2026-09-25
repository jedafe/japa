-- 000245_rag_knowledge_layer.down.sql
-- Reverses 000245_rag_knowledge_layer.up.sql.
-- Drops the content_embeddings table and removes the seeded rag.* settings
-- and ai.rag.* permission nodes.

BEGIN;

-- Revoke ai.rag.* grants from super_admin.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND rp.permission_id = p.id
  AND p.key IN ('ai.rag.manage', 'ai.rag.reindex');

-- Remove ai.rag.* permission nodes.
DELETE FROM permissions WHERE key IN ('ai.rag.manage', 'ai.rag.reindex');

-- Remove rag.* system_settings for the default tenant.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('rag.enabled', 'rag.embedding_model', 'rag.retrieval_top_k', 'rag.chunk_max_tokens');

-- Drop the content_embeddings table (cascades indexes).
DROP TABLE IF EXISTS content_embeddings;

COMMIT;
