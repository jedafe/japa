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

// PoolConfig holds the tuning parameters applied to every pgxpool.
// Values are sourced from JC_* environment variables via internal/config;
// zero values fall back to the defaults in defaultPoolConfig.
type PoolConfig struct {
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// defaultPoolConfig returns the connection pool tuning used when the caller
// does not supply explicit values (or supplies zeroes). These defaults are
// chosen for a typical small-to-medium deployment.
func defaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConns:        25,
		MinConns:        5,
		MaxConnLifetime: 30 * time.Minute,
		MaxConnIdleTime: 5 * time.Minute,
	}
}

// resolvePoolConfig fills zero-valued fields with defaults so callers can
// pass a partially-specified PoolConfig (e.g. from env vars where unset
// means "use the default").
func resolvePoolConfig(cfg PoolConfig) PoolConfig {
	out := defaultPoolConfig()
	if cfg.MaxConns > 0 {
		out.MaxConns = cfg.MaxConns
	}

	if cfg.MinConns > 0 {
		out.MinConns = cfg.MinConns
	}

	if cfg.MaxConnLifetime > 0 {
		out.MaxConnLifetime = cfg.MaxConnLifetime
	}

	if cfg.MaxConnIdleTime > 0 {
		out.MaxConnIdleTime = cfg.MaxConnIdleTime
	}

	return out
}

// New creates a DB with writer and reader pools pointing at the same DSN,
// using default pool tuning. Until a read replica exists, both pools connect
// to the same database. Callers must call Close when the DB is no longer
// needed to release connections back to PostgreSQL.
func New(ctx context.Context, dsn string) (*DB, error) {
	return NewWithReplica(ctx, dsn, dsn, PoolConfig{})
}

// NewWithConfig creates a DB with writer and reader pools pointing at the
// same DSN, using the supplied PoolConfig. Zero-valued fields in cfg fall
// back to defaults. Use this when the caller has pool tuning from JC_*
// environment variables.
func NewWithConfig(ctx context.Context, dsn string, cfg PoolConfig) (*DB, error) {
	return NewWithReplica(ctx, dsn, dsn, cfg)
}

// NewWithReplica creates a DB with separate DSNs for the writer and reader.
// Use this when a read replica has been provisioned. The writer DSN must
// point at the primary; the reader DSN should point at the replica. Zero-
// valued fields in cfg fall back to defaults.
func NewWithReplica(ctx context.Context, writerDSN, readerDSN string, cfg PoolConfig) (*DB, error) {
	resolved := resolvePoolConfig(cfg)

	writer, err := createPool(ctx, writerDSN, resolved)
	if err != nil {
		return nil, fmt.Errorf("creating writer pool: %w", err)
	}

	reader, err := createPool(ctx, readerDSN, resolved)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("creating reader pool: %w", err)
	}

	return &DB{Writer: writer, Reader: reader}, nil
}

// createPool builds a single pgxpool.Pool with the given DSN and tuning.
// The pool is created lazily — connections are opened on demand — but we
// ping once to fail fast if the database is unreachable.
func createPool(ctx context.Context, dsn string, cfg PoolConfig) (*pgxpool.Pool, error) {
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
	"contents":                  {},
	"feeds":                     {},
	"notifications":             {},
	"users":                     {},
	"user_profiles":             {},
	"spaces":                    {},
	"space_members":             {},
	"comments":                  {},
	"reactions":                 {},
	"bookmarks":                 {},
	"follows":                   {},
	"tickets":                   {},
	"payments":                  {},
	"media":                     {},
	"audit_logs":                {},
	"account_deletion_requests": {},
	"consent_records":           {},
	"dm_messages":               {},
	"dm_conversations":          {},
	"webhook_deliveries":        {},
	"webhook_endpoints":         {},
	"trending_snapshots":        {},
	"search_analytics":          {},
	"academy_tracks":            {},
	"academy_lessons":           {},
	"academy_modules":           {},
	"academy_lesson_progress":   {},
	"academy_certificates":      {},
	"academy_enrollments":       {},
	"organizations":             {},
	"organization_members":      {},
	"moderation_alerts":         {},
	"user_moderation_actions":   {},
	"content_reports":           {},
	"coupon_events":             {},
	"coupons":                   {},
	"affiliate_link_events":     {},
	"user_affiliate_links":      {},
	"affiliate_program_members": {},
	// v2 tables (sections 60–78)
	"ai_personas":             {},
	"assessment_results":      {},
	"badge_definitions":       {},
	"challenge_definitions":   {},
	"checklist_progress":      {},
	"content_embeddings":      {},
	"content_imports":         {},
	"newsletter_subscribers":  {},
	"referral_codes":          {},
	"referral_programs":       {},
	"referrals":               {},
	"remittance_providers":    {},
	"remittance_rate_alerts":  {},
	"remittance_rates":        {},
	"saved_scholarships":      {},
	"studio_jobs":             {},
	"travel_providers":        {},
	"user_badges":             {},
	"user_challenge_progress": {},
	// v2 community tables (sections 79–89, §98)
	"achievement_meta":        {},
	"calendar_entries":        {},
	"calendar_saves":          {},
	"calendar_subscriptions":  {},
	"community_organisations": {},
	"digest_editions":         {},
	// mentorship_connections was dropped by 000824 — connection state lives
	// in service_bookings now, which has no vacuum allow-list entry.
	"mutual_aid_posts":          {},
	"planning_group_checkins":   {},
	"planning_group_members":    {},
	"planning_group_milestones": {},
	"planning_groups":           {},
	"scam_alert_subscriptions":  {},
	"scam_reports":              {},
	"service_listing_meta":      {},
	"story_meta":                {},
	"visa_processing_stats":     {},
	"visa_timeline_milestones":  {},
	"visa_timelines":            {},
	"wiki_pages":                {},
	"wiki_revisions":            {},
	// v2 functional tool tables (sections 90–97, §§99–101)
	"appointment_alerts":            {},
	"arrival_checklist_guides":      {},
	"arrival_checklist_items":       {},
	"attestation_community_updates": {},
	"attestation_pathways":          {},
	"credential_community_notes":    {},
	"credential_pathways":           {},
	"credential_professions":        {},
	"cv_format_sessions":            {},
	"cv_standards":                  {},
	"housing_guides":                {},
	"language_score_stories":        {},
	"language_test_requirements":    {},
	"language_test_resources":       {},
	"language_tests":                {},
	"monitored_embassies":           {},
	"tax_obligation_guides":         {},
	"tax_treaty_pairs":              {},
	"visa_routes":                   {},
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
