-- 000880_spec_doc_partial_hnsw.up.sql — partial HNSW index scoped to
-- entity_type='spec_doc' (§121, ADR-131).
--
-- The shared idx_content_embeddings_hnsw graph traverses the global nearest
-- neighbors before the entity_type filter is applied; a spec query can
-- exhaust hnsw.ef_search on non-spec rows and return fewer than topK spec
-- hits (recall starvation — especially pronounced when the spec corpus and
-- user content were embedded by different models/spaces). A partial index
-- contains only spec_doc vectors, giving spec queries a self-contained
-- nearest-neighbor graph.
--
-- Ref: japaconnect-engineering/sections/121-engineering-spec-corpus.md

CREATE INDEX idx_content_embeddings_spec_doc_hnsw
    ON public.content_embeddings USING hnsw (embedding public.vector_cosine_ops)
    WHERE entity_type = 'spec_doc';
