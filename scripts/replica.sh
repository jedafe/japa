#!/bin/bash
# scripts/replica.sh — manage the local PostgreSQL read replica.
#
# Usage:
#   ./scripts/replica.sh start    # start the replica on port 5434
#   ./scripts/replica.sh stop     # stop the replica
#   ./scripts/replica.sh status   # show replication status
#   ./scripts/replica.sh reset    # wipe and re-clone from primary
#
# The replica data directory is /home/linuxbrew/.linuxbrew/var/postgresql@15-replica
# and streams WAL from the primary at localhost:5432.

set -e

PG_BIN="/home/linuxbrew/.linuxbrew/bin"
PG_CTL="$PG_BIN/pg_ctl"
PSQL="$PG_BIN/psql"
PG_BASEBACKUP="$PG_BIN/pg_basebackup"
REPLICA_DIR="/home/linuxbrew/.linuxbrew/var/postgresql@15-replica"
PRIMARY_HOST="localhost"
PRIMARY_PORT="5432"
REPLICA_PORT="5434"
REPLICA_USER="replicator"
REPLICA_PASSWORD="japa_replica"

cmd="${1:-status}"

case "$cmd" in
  start)
    if "$PG_CTL" -D "$REPLICA_DIR" status >/dev/null 2>&1; then
      echo "replica: already running"
      exit 0
    fi
    if [ ! -s "$REPLICA_DIR/PG_VERSION" ]; then
      echo "replica: data directory not initialized. Run: $0 reset"
      exit 1
    fi
    echo "replica: starting on port $REPLICA_PORT"
    "$PG_CTL" -D "$REPLICA_DIR" -l "$REPLICA_DIR/logfile" start
    ;;

  stop)
    echo "replica: stopping"
    "$PG_CTL" -D "$REPLICA_DIR" stop -m fast || true
    ;;

  restart)
    "$0" stop
    "$0" start
    ;;

  status)
    echo "=== Replica process ==="
    "$PG_CTL" -D "$REPLICA_DIR" status || true
    echo ""
    echo "=== Is replica in recovery? ==="
    "$PSQL" "postgresql://$PRIMARY_HOST:$REPLICA_PORT/postgres" -c "SELECT pg_is_in_recovery();" 2>/dev/null || echo "cannot connect to replica"
    echo ""
    echo "=== Replication lag from primary ==="
    "$PSQL" "postgresql://$PRIMARY_HOST:$PRIMARY_PORT/postgres" -c "
      SELECT usename, state,
             pg_size_pretty(pg_wal_lsn_diff(sent_lsn, replay_lsn)) AS lag,
             replay_lsn
      FROM pg_stat_replication;
    " 2>/dev/null || echo "cannot connect to primary"
    ;;

  reset)
    echo "replica: stopping (if running)"
    "$PG_CTL" -D "$REPLICA_DIR" stop -m fast || true
    echo "replica: wiping data directory"
    rm -rf "$REPLICA_DIR"
    mkdir -p "$REPLICA_DIR"
    chmod 700 "$REPLICA_DIR"
    echo "replica: cloning primary with pg_basebackup"
    PGPASSWORD="$REPLICA_PASSWORD" "$PG_BASEBACKUP" \
      -h "$PRIMARY_HOST" \
      -p "$PRIMARY_PORT" \
      -U "$REPLICA_USER" \
      -D "$REPLICA_DIR" \
      -Fp -Xs -P -R
    echo "port = $REPLICA_PORT" >> "$REPLICA_DIR/postgresql.conf"
    echo "replica: clone complete. Run: $0 start"
    ;;

  *)
    echo "Usage: $0 {start|stop|restart|status|reset}"
    exit 1
    ;;
esac
