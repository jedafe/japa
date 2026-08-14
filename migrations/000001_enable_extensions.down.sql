-- Down migration for 000001_enable_extensions.sql
-- Drops the PostgreSQL extensions enabled by the up migration.
DROP EXTENSION IF EXISTS "pg_trgm";
DROP EXTENSION IF EXISTS "pg_stat_statements";
DROP EXTENSION IF EXISTS "vector";
DROP EXTENSION IF EXISTS "uuid-ossp";
