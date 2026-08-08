// Package digest implements the Community Digest Widget (§84, ADR-092).
//
// Periodic round-ups of community activity (new posts, trending threads,
// upcoming events, notable wiki edits) surfaced as a widget on the space
// home and optionally delivered via email. Editions move through a
// draft → scheduled → published lifecycle and are reviewed by a moderator+
// before publication.
//
// Ref: japaconnect-engineering/sections/84-community-digest.md
//      japaconnect-engineering/adr/ADR-092-community-digest.md
package digest

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Store is the data-access layer for digest_editions.
type Store struct {
	pool *db.DB
}

// NewStore creates a digest store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// DigestEdition represents a single digest_editions row.
type DigestEdition struct {
	ID           pgtype.UUID
	TenantID     pgtype.UUID
	SpaceID      pgtype.UUID
	Cadence      string
	Status       string // draft|scheduled|published
	ScheduledFor pgtype.Timestamptz
	PublishedAt  pgtype.Timestamptz
	Content      []byte // jsonb
	EmailSent    bool
	EmailSentAt  pgtype.Timestamptz
	GeneratedBy  pgtype.UUID
	ReviewedBy   pgtype.UUID
	CreatedAt    pgtype.Timestamptz
}

// CreateEdition inserts a new digest edition and returns its id.
func (s *Store) CreateEdition(ctx context.Context, edition DigestEdition) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, `
INSERT INTO digest_editions (tenant_id, space_id, cadence, status, scheduled_for,
    content, generated_by)
VALUES ($1, $2, $3, COALESCE($4, 'draft'), $5, $6, $7)
RETURNING id
`, edition.TenantID, edition.SpaceID, edition.Cadence, edition.Status,
		edition.ScheduledFor, edition.Content, edition.GeneratedBy,
	).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("digest: create edition: %w", err)
	}
	return id, nil
}

// GetEdition returns a single digest edition by id within a tenant.
func (s *Store) GetEdition(ctx context.Context, id, tenantID pgtype.UUID) (DigestEdition, error) {
	const q = `
SELECT id, tenant_id, space_id, cadence, status, scheduled_for, published_at,
       content, email_sent, email_sent_at, generated_by, reviewed_by, created_at
FROM digest_editions
WHERE id = $1 AND tenant_id = $2
LIMIT 1
`
	var e DigestEdition
	err := s.pool.Reader.QueryRow(ctx, q, id, tenantID).Scan(
		&e.ID, &e.TenantID, &e.SpaceID, &e.Cadence, &e.Status,
		&e.ScheduledFor, &e.PublishedAt, &e.Content, &e.EmailSent,
		&e.EmailSentAt, &e.GeneratedBy, &e.ReviewedBy, &e.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return DigestEdition{}, pgx.ErrNoRows
		}
		return DigestEdition{}, fmt.Errorf("digest: get edition: %w", err)
	}
	return e, nil
}

// GetLatestPublished returns the most recently published edition for a tenant
// (and optionally a space). If spaceID is not valid, the tenant-wide digest
// is returned.
func (s *Store) GetLatestPublished(ctx context.Context, tenantID, spaceID pgtype.UUID) (DigestEdition, error) {
	var (
		q    strings.Builder
		args []any
	)
	q.WriteString(`
SELECT id, tenant_id, space_id, cadence, status, scheduled_for, published_at,
       content, email_sent, email_sent_at, generated_by, reviewed_by, created_at
FROM digest_editions
WHERE tenant_id = $1 AND status = 'published'
`)
	args = append(args, tenantID)
	argIdx := 2
	if spaceID.Valid {
		q.WriteString(fmt.Sprintf(" AND space_id = $%d", argIdx))
		args = append(args, spaceID)
		argIdx++
	}
	q.WriteString(fmt.Sprintf(" ORDER BY published_at DESC LIMIT 1"))

	var e DigestEdition
	err := s.pool.Reader.QueryRow(ctx, q.String(), args...).Scan(
		&e.ID, &e.TenantID, &e.SpaceID, &e.Cadence, &e.Status,
		&e.ScheduledFor, &e.PublishedAt, &e.Content, &e.EmailSent,
		&e.EmailSentAt, &e.GeneratedBy, &e.ReviewedBy, &e.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return DigestEdition{}, pgx.ErrNoRows
		}
		return DigestEdition{}, fmt.Errorf("digest: get latest published: %w", err)
	}
	return e, nil
}

// ListEditionsFilter holds filter parameters for the admin edition list.
type ListEditionsFilter struct {
	TenantID pgtype.UUID
	SpaceID  pgtype.UUID
	Status   string
	Limit    int32
	Offset   int32
}

// ListEditions returns all digest editions for admin management (paginated).
func (s *Store) ListEditions(ctx context.Context, f ListEditionsFilter) ([]DigestEdition, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	if f.SpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("space_id = $%d", argIdx))
		args = append(args, f.SpaceID)
		argIdx++
	}
	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	listQ := fmt.Sprintf(`
SELECT id, tenant_id, space_id, cadence, status, scheduled_for, published_at,
       content, email_sent, email_sent_at, generated_by, reviewed_by, created_at
FROM digest_editions
WHERE %s
ORDER BY created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("digest: list editions: %w", err)
	}
	defer rows.Close()

	var out []DigestEdition
	for rows.Next() {
		var e DigestEdition
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.SpaceID, &e.Cadence, &e.Status,
			&e.ScheduledFor, &e.PublishedAt, &e.Content, &e.EmailSent,
			&e.EmailSentAt, &e.GeneratedBy, &e.ReviewedBy, &e.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("digest: scan edition: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("digest: iterate editions: %w", err)
	}

	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM digest_editions WHERE %s`, whereClause)
	countArgs := args[:len(args)-2]
	var total int64
	if err := s.pool.Reader.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("digest: count editions: %w", err)
	}

	return out, total, nil
}

// UpdateEdition updates the content of a draft (unpublished) edition.
func (s *Store) UpdateEdition(ctx context.Context, id, tenantID pgtype.UUID, content []byte) error {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE digest_editions
SET content = $3
WHERE id = $1 AND tenant_id = $2 AND status = 'draft'
`, id, tenantID, content)
	if err != nil {
		return fmt.Errorf("digest: update edition: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// PublishEdition marks an edition as published and stamps the reviewer.
func (s *Store) PublishEdition(ctx context.Context, id, tenantID, reviewerID pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE digest_editions
SET status = 'published', published_at = now(), reviewed_by = $3
WHERE id = $1 AND tenant_id = $2 AND status IN ('draft', 'scheduled')
`, id, tenantID, reviewerID)
	if err != nil {
		return fmt.Errorf("digest: publish edition: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
