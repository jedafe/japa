#!/bin/bash
# docker/postgres-replica-entrypoint.sh
# Initializes a PostgreSQL read replica from the primary and starts it
# in standby mode. Used by the postgres-replica service in docker-compose.yml.
#
# On first boot (empty data directory):
#   1. Wait for the primary to be ready
#   2. Run pg_basebackup to clone the primary's data
#   3. Create standby.signal and configure primary_conninfo
#   4. Start postgres in standby mode
#
# On subsequent boots (data directory exists):
#   Just start postgres — it resumes streaming from where it left off.

set -e

PRIMARY_HOST="${PRIMARY_HOST:-postgres}"
PRIMARY_PORT="${PRIMARY_PORT:-5432}"
REPLICA_USER="${REPLICA_USER:-replicator}"
REPLICA_PASSWORD="${REPLICA_PASSWORD:-japa_replica}"
PGDATA="/var/lib/postgresql/data"

# If data directory already has content, skip basebackup and just start.
if [ -s "$PGDATA/PG_VERSION" ]; then
    echo "postgres-replica: data directory exists, starting in standby mode"
    exec docker-entrypoint.sh postgres
fi

# Wait for the primary to be ready.
echo "postgres-replica: waiting for primary at $PRIMARY_HOST:$PRIMARY_PORT"
until pg_isready -h "$PRIMARY_HOST" -p "$PRIMARY_PORT" -U "$REPLICA_USER" >/dev/null 2>&1; do
    sleep 1
done
echo "postgres-replica: primary is ready"

# Run pg_basebackup to clone the primary.
echo "postgres-replica: running pg_basebackup from primary"
PGPASSWORD="$REPLICA_PASSWORD" pg_basebackup \
    -h "$PRIMARY_HOST" \
    -p "$PRIMARY_PORT" \
    -U "$REPLICA_USER" \
    -D "$PGDATA" \
    -Fp -Xs -P -R

# -R flag creates standby.signal and writes primary_conninfo to
# postgresql.auto.conf, so the replica will start in standby mode
# and connect back to the primary for WAL streaming.

echo "postgres-replica: basebackup complete, starting in standby mode"
exec docker-entrypoint.sh postgres