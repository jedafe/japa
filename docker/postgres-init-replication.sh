#!/bin/bash
# docker/postgres-init-replication.sh
# Allows the postgres-replica service to stream WAL from the primary. The
# image's POSTGRES_HOST_AUTH_METHOD rule ("host all all all ...") does not
# cover the special "replication" database, so pg_basebackup is rejected
# without this line. Runs once on first boot (empty data directory).
set -e
echo "host replication replicator all scram-sha-256" >> "$PGDATA/pg_hba.conf"
