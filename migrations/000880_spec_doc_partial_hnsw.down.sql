-- 000880_spec_doc_partial_hnsw.down.sql — drop the spec_doc partial HNSW index.
DROP INDEX IF EXISTS public.idx_content_embeddings_spec_doc_hnsw;
