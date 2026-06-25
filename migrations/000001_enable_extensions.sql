-- 000001_enable_extensions.sql
-- Enable PostgreSQL extensions required by JapaConnect.
-- uuid-ossp: legacy UUID generation (gen_random_uuid is built into PG13+,
--   kept for compatibility with tooling that references it).
-- pgvector: vector similarity search for AI embeddings.
-- pg_stat_statements: query performance statistics for observability.
-- pg_trgm: trigram-based fuzzy text search for autocomplete and matching.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "vector";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
