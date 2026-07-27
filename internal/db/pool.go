// Package db provides the PostgreSQL connection pool for JapaConnect.
//
// The DB struct holds two pgxpool.Pool connections: a Writer used for all
// write operations and a Reader used for read queries. Until a read replica
// is provisioned, both point to the same DSN. When a replica is added, only
// the Reader's DSN needs to change — application code that reads from
// db.Reader will automatically use the replica.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB holds the writer and reader connection pools.
// Use Writer for INSERT/UPDATE/DELETE and Reader for SELECT queries.
type DB struct {
	Writer *pgxpool.Pool
	Reader *pgxpool.Pool
}

// poolConfig holds the tuning parameters applied to every pgxpool.
// These values are chosen for a typical small-to-medium deployment and can
// be adjusted via environment variables in a future iteration.
type poolConfig struct {
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// defaultPoolConfig returns the connection pool tuning used for both writer
// and reader pools.
func defaultPoolConfig() poolConfig {
	return poolConfig{
		MaxConns:        25,
		MinConns:        5,
		MaxConnLifetime: 30 * time.Minute,
		MaxConnIdleTime: 5 * time.Minute,
	}
}

// New creates a DB with writer and reader pools pointing at the same DSN.
// Until a read replica exists, both pools connect to the same database.
// Callers must call Close when the DB is no longer needed to release
// connections back to PostgreSQL.
func New(ctx context.Context, dsn string) (*DB, error) {
	return NewWithReplica(ctx, dsn, dsn)
}

// NewWithReplica creates a DB with separate DSNs for the writer and reader.
// Use this when a read replica has been provisioned. The writer DSN must
// point at the primary; the reader DSN should point at the replica.
func NewWithReplica(ctx context.Context, writerDSN, readerDSN string) (*DB, error) {
	cfg := defaultPoolConfig()

	writer, err := createPool(ctx, writerDSN, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating writer pool: %w", err)
	}

	reader, err := createPool(ctx, readerDSN, cfg)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("creating reader pool: %w", err)
	}

	return &DB{Writer: writer, Reader: reader}, nil
}

// createPool builds a single pgxpool.Pool with the given DSN and tuning.
// The pool is created lazily — connections are opened on demand — but we
// ping once to fail fast if the database is unreachable.
func createPool(ctx context.Context, dsn string, cfg poolConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing DSN: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return pool, nil
}

// Ping checks that both writer and reader pools can reach the database.
// Use this for readiness checks — it returns an error if either pool
// cannot establish a connection within the context deadline.
func (db *DB) Ping(ctx context.Context) error {
	if err := db.Writer.Ping(ctx); err != nil {
		return fmt.Errorf("pinging writer pool: %w", err)
	}

	if err := db.Reader.Ping(ctx); err != nil {
		return fmt.Errorf("pinging reader pool: %w", err)
	}

	return nil
}

// Close releases all connections in both pools. It is safe to call multiple
// times — pgxpool.Close is idempotent.
func (db *DB) Close() {
	if db.Writer != nil {
		db.Writer.Close()
	}

	if db.Reader != nil {
		db.Reader.Close()
	}
}

// allowedVacuumTables is the allow-list of tables that may be targeted by
// Vacuum. VACUUM cannot use a parameterized table name, so the caller
// passes a name that is validated against this set before execution.
var allowedVacuumTables = map[string]struct{}{
	"contents":              {},
	"feeds":                 {},
	"notifications":         {},
	"users":                 {},
	"user_profiles":         {},
	"spaces":                {},
	"space_members":         {},
	"comments":              {},
	"reactions":             {},
	"bookmarks":             {},
	"follows":               {},
	"tickets":               {},
	"payments":              {},
	"media":                 {},
	"audit_logs":            {},
	"account_deletion_requests": {},
	"consent_records":       {},
	"dm_messages":           {},
	"dm_conversations":      {},
	"webhook_deliveries":    {},
	"webhook_endpoints":     {},
	"trending_snapshots":    {},
	"search_analytics":      {},
	"learning_tracks":       {},
	"learning_lessons":      {},
	"learning_modules":      {},
	"lesson_progress":       {},
	"learning_certificates": {},
	"enrollments":           {},
	"organizations":         {},
	"organization_members":  {},
	"moderation_alerts":     {},
	"user_moderation_actions": {},
	"content_reports":       {},
	"coupon_events":         {},
	"coupons":               {},
	"affiliate_link_events": {},
	"user_affiliate_links":  {},
	"affiliate_program_members": {},
}

// Vacuum runs a VACUUM (or VACUUM FULL if full is true) on the given table.
// If table is empty, the entire database is vacuumed. VACUUM cannot run
// inside a transaction and table names cannot be parameterized, so the
// table name is validated against an allow-list before execution.
// This is the only place in the codebase that executes raw VACUUM SQL.
func (db *DB) Vacuum(ctx context.Context, table string, full bool) error {
	stmt := "VACUUM"
	if full {
		stmt = "VACUUM FULL"
	}
	if table != "" {
		if _, ok := allowedVacuumTables[table]; !ok {
			return fmt.Errorf("vacuum: table %q is not in the allow-list", table)
		}
		stmt += " " + table
	}
	if _, err := db.Writer.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	return nil
}
