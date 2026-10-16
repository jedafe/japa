-- 000874_rag_spec_corpus.down.sql — revoke the agent.spec_search grant and
-- drop the spec-corpus registry. Purges any spec_doc embedding rows so a
-- rollback leaves no orphaned private chunks behind.

DELETE FROM content_embeddings WHERE entity_type = 'spec_doc';

DROP TABLE IF EXISTS rag_spec_sources;

DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'agent.spec_search');

DELETE FROM permissions WHERE key = 'agent.spec_search';
