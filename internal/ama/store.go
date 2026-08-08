// Package ama implements the "Ask an Expat" thread format (§88, ADR-096).
//
// AMA threads are forum_threads with a non-null ama_config JSONB column.
// This package provides the AMA-specific listing and retrieval endpoints
// that filter forum_threads by ama_config IS NOT NULL. Thread creation
// and update go through the existing forum handlers — the ama_config is
// set via the thread creation payload when thread_type = 'ama'.
//
// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
//      japaconnect-engineering/adr/ADR-096-ask-an-expat.md
package ama

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Store is the data-access layer for AMA threads (forum_threads with
// ama_config IS NOT NULL).
type Store struct {
	pool *db.DB
}

// NewStore creates an AMA store.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// AMAThread is a forum thread with AMA config, returned by list/get queries.
type AMAThread struct {
	ID           pgtype.UUID
	TenantID     pgtype.UUID
	Title        string
	Slug         string
	Body         string
	Status       string
	AuthorID     pgtype.UUID
	AuthorName   *string
	AuthorAvatar *string
	AmaConfig    []byte // JSONB
	CreatedAt    pgtype.Timestamptz
	UpdatedAt    pgtype.Timestamptz
}

// ListActive returns AMA threads that are still open (closes_at IS NULL or
// closes_at > now()), ordered by most recent first.
func (s *Store) ListActive(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]AMAThread, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	const q = `
SELECT ft.id, ft.tenant_id, ft.title, ft.slug, ft.body, ft.status,
       ft.user_id, u.username, up.avatar_url,
       ft.ama_config, ft.created_at, ft.updated_at
FROM forum_threads ft
LEFT JOIN users u ON u.id = ft.user_id
LEFT JOIN user_profiles up ON up.user_id = ft.user_id
WHERE ft.tenant_id = $1 AND ft.ama_config IS NOT NULL
  AND ft.deleted_at IS NULL
  AND (ft.ama_config->>'closes_at' IS NULL
       OR (ft.ama_config->>'closes_at')::timestamptz > now())
ORDER BY ft.created_at DESC
LIMIT $2 OFFSET $3
`
	rows, err := s.pool.Reader.Query(ctx, q, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("ama: list active: %w", err)
	}
	defer rows.Close()

	var out []AMAThread
	for rows.Next() {
		var t AMAThread
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.Title, &t.Slug, &t.Body, &t.Status,
			&t.AuthorID, &t.AuthorName, &t.AuthorAvatar,
			&t.AmaConfig, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("ama: scan: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("ama: iterate: %w", err)
	}

	var total int64
	if err := s.pool.Reader.QueryRow(ctx, `
SELECT COUNT(*) FROM forum_threads
WHERE tenant_id = $1 AND ama_config IS NOT NULL AND deleted_at IS NULL
  AND (ama_config->>'closes_at' IS NULL
       OR (ama_config->>'closes_at')::timestamptz > now())
`, tenantID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ama: count: %w", err)
	}

	return out, total, nil
}

// GetByID returns a single AMA thread by its ID.
func (s *Store) GetByID(ctx context.Context, id, tenantID pgtype.UUID) (AMAThread, error) {
	const q = `
SELECT ft.id, ft.tenant_id, ft.title, ft.slug, ft.body, ft.status,
       ft.user_id, u.username, up.avatar_url,
       ft.ama_config, ft.created_at, ft.updated_at
FROM forum_threads ft
LEFT JOIN users u ON u.id = ft.user_id
LEFT JOIN user_profiles up ON up.user_id = ft.user_id
WHERE ft.id = $1 AND ft.tenant_id = $2 AND ft.ama_config IS NOT NULL
  AND ft.deleted_at IS NULL
LIMIT 1
`
	var t AMAThread
	err := s.pool.Reader.QueryRow(ctx, q, id, tenantID).Scan(
		&t.ID, &t.TenantID, &t.Title, &t.Slug, &t.Body, &t.Status,
		&t.AuthorID, &t.AuthorName, &t.AuthorAvatar,
		&t.AmaConfig, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AMAThread{}, pgx.ErrNoRows
		}
		return AMAThread{}, fmt.Errorf("ama: get by id: %w", err)
	}
	return t, nil
}

// CloseAMA sets the closes_at field in ama_config to now(), closing the AMA.
func (s *Store) CloseAMA(ctx context.Context, threadID, tenantID pgtype.UUID) error {
	_, err := s.pool.Writer.Exec(ctx, `
UPDATE forum_threads
SET ama_config = jsonb_set(ama_config, '{closes_at}', to_jsonb(now()::timestamptz), true),
    updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND ama_config IS NOT NULL AND deleted_at IS NULL
`, threadID, tenantID)
	if err != nil {
		return fmt.Errorf("ama: close: %w", err)
	}
	return nil
}

// CheckCooldown returns true if the user has created an AMA within the
// cooldown period (default 30 days, configurable via ama_cooldown_days).
func (s *Store) CheckCooldown(ctx context.Context, userID, tenantID pgtype.UUID, cooldownDays int) (bool, error) {
	if cooldownDays <= 0 {
		return false, nil
	}
	var exists bool
	err := s.pool.Reader.QueryRow(ctx, `
SELECT EXISTS(
    SELECT 1 FROM forum_threads
    WHERE user_id = $1 AND tenant_id = $2 AND ama_config IS NOT NULL
      AND deleted_at IS NULL
      AND created_at > now() - ($3 || ' days')::interval
)
`, userID, tenantID, fmt.Sprintf("%d", cooldownDays)).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("ama: check cooldown: %w", err)
	}
	return exists, nil
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// suppress unused import warning for strings (used in future query builders).
var _ = strings.TrimSpace
