-- docker/postgres-init.sql
-- Create the additional databases that JapaConnect services depend on.
-- The primary "japa" database is created by the POSTGRES_DB environment
-- variable in docker-compose.yml. This script runs once on first boot
-- (when the PostgreSQL data directory is initialized) to create the
-- extra databases that Honcho and Glitchtip need.

CREATE DATABASE honcho;
CREATE DATABASE glitchtip;

-- Replication user for the read replica (ADR-004 amendment performance tuning).
-- The replica uses this account to stream WAL from the primary.
CREATE ROLE replicator WITH REPLICATION LOGIN PASSWORD 'japa_replica';
