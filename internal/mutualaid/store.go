// Package mutualaid implements the Mutual Aid Board (§85, ADR-093).
//
// Members post needs, offers, and skill swaps scoped to a destination
// space (country) and optional city space. Posts expire after 30 days
// and can be renewed, marked fulfilled, or soft-deleted by the author.
// Admins moderate via the status field (active, expired, moderated,
// fulfilled).
//
// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
//
//	japaconnect-engineering/adr/ADR-093-mutual-aid-board.md
package mutualaid

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Store is the data-access layer for mutual_aid_posts. It uses direct pgx
// queries because mutual_aid_posts is a new table without sqlc-generated
// queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a mutual aid store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// Post types and statuses (mirrors the DB enum/check constraints).
const (
	PostTypeNeed      = "need"
	PostTypeOffer     = "offer"
	PostTypeSkillSwap = "skill_swap"

	StatusActive    = "active"
	StatusExpired   = "expired"
	StatusRemoved   = "removed"
	StatusFulfilled = "fulfilled"

	// Categories per §85 (mirrors the CHECK constraint added in 000410).
	CategoryHousing   = "housing"
	CategoryTransport = "transport"
	CategoryDocuments = "documents"
	CategoryJobSearch = "job_search"
	CategoryLanguage  = "language"
	CategorySocial    = "social"
	CategoryOther     = "other"
)

// validCategories is the set of categories allowed by §85 and enforced by
// the mutual_aid_posts_category_check constraint (migration 000410).
var validCategories = map[string]struct{}{
	CategoryHousing:   {},
	CategoryTransport: {},
	CategoryDocuments: {},
	CategoryJobSearch: {},
	CategoryLanguage:  {},
	CategorySocial:    {},
	CategoryOther:     {},
}

// IsValidCategory reports whether c is one of the §85 categories.
func IsValidCategory(c string) bool {
	_, ok := validCategories[c]
	return ok
}

// Post is a single mutual_aid_posts row.
type Post struct {
	ID                 pgtype.UUID
	TenantID           pgtype.UUID
	UserID             pgtype.UUID
	PostType           string
	Category           *string
	Title              string
	Description        *string
	DestinationSpaceID pgtype.UUID
	CitySpaceID        pgtype.UUID
	ExpiresAt          pgtype.Timestamptz
	IsFulfilled        bool
	Status             string
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
	DeletedAt          pgtype.Timestamptz
}

// PostRow is the joined result of mutual_aid_posts + spaces + users,
// returned by list and get queries.
type PostRow struct {
	Post
	AuthorName    *string
	AuthorAvatar  *string
	DestSpaceName *string
	DestSpaceCode *string
	CitySpaceName *string
	CitySpaceCode *string
}

// CreatePostInput holds the fields for inserting a new post.
type CreatePostInput struct {
	TenantID           pgtype.UUID
	UserID             pgtype.UUID
	PostType           string
	Category           *string
	Title              string
	Description        *string
	DestinationSpaceID pgtype.UUID
	CitySpaceID        pgtype.UUID
	ExpiresAt          pgtype.Timestamptz
}

// UpdatePostInput holds the mutable fields for updating an existing post.
// Only non-nil fields are applied.
type UpdatePostInput struct {
	Category    *string
	Title       *string
	Description *string
}

// ListPostsFilter holds the filter parameters for listing active posts.
type ListPostsFilter struct {
	TenantID           pgtype.UUID
	PostType           string      // optional: need | offer | skill_swap
	Category           string      // optional
	DestinationSpaceID pgtype.UUID // optional
	CitySpaceID        pgtype.UUID // optional
	Limit              int32
	Offset             int32
}

// AdminListPostsFilter holds filter parameters for the admin moderation list.
type AdminListPostsFilter struct {
	TenantID           pgtype.UUID
	PostType           string // optional
	Status             string // optional
	DestinationSpaceID pgtype.UUID
	CitySpaceID        pgtype.UUID
	Q                  string // optional: search title
	Limit              int32
	Offset             int32
}

// CreatePost inserts a new mutual_aid_posts row.
func (s *Store) CreatePost(ctx context.Context, input CreatePostInput) (Post, error) {
	const q = `
INSERT INTO mutual_aid_posts (
    user_id, post_type, category, title, description,
    destination_space_id, city_space_id, expires_at, is_fulfilled, status
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, false, 'active')
RETURNING id, user_id, post_type, category, title, description,
          destination_space_id, city_space_id, expires_at, is_fulfilled,
          status, created_at, updated_at, deleted_at
`

	var p Post

	err := s.pool.Writer.QueryRow(
		ctx, q,
		input.TenantID, input.UserID, input.PostType, input.Category,
		input.Title, input.Description, input.DestinationSpaceID,
		input.CitySpaceID, input.ExpiresAt,
	).Scan(
		&p.ID, &p.TenantID, &p.UserID, &p.PostType, &p.Category, &p.Title,
		&p.Description, &p.DestinationSpaceID, &p.CitySpaceID, &p.ExpiresAt,
		&p.IsFulfilled, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)
	if err != nil {
		return Post{}, fmt.Errorf("mutualaid: create post: %w", err)
	}

	return p, nil
}

// GetPost returns a single publicly-visible post by id. A post is publicly
// visible only when not soft-deleted AND not moderated/removed (BUG-009).
// Removed posts are reachable via the admin endpoints, not the public one.
func (s *Store) GetPost(ctx context.Context, id, tenantID pgtype.UUID) (PostRow, error) {
	const q = `
SELECT m.id, m.user_id, m.post_type, m.category, m.title,
       m.description, m.destination_space_id, m.city_space_id, m.expires_at,
       m.is_fulfilled, m.status, m.created_at, m.updated_at, m.deleted_at,
       u.username AS author_name, up.avatar_url AS author_avatar,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       cs.name AS city_space_name, cs.geo_code AS city_space_code
FROM mutual_aid_posts m
LEFT JOIN users u ON u.id = m.user_id
LEFT JOIN user_profiles up ON up.user_id = m.user_id
LEFT JOIN spaces ds ON ds.id = m.destination_space_id
LEFT JOIN spaces cs ON cs.id = m.city_space_id
WHERE m.id = $1 AND m.deleted_at IS NULL
  AND m.status <> 'removed'
LIMIT 1
`

	var r PostRow

	err := s.pool.Reader.QueryRow(ctx, q, id, tenantID).Scan(
		&r.ID, &r.TenantID, &r.UserID, &r.PostType, &r.Category, &r.Title,
		&r.Description, &r.DestinationSpaceID, &r.CitySpaceID, &r.ExpiresAt,
		&r.IsFulfilled, &r.Status, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
		&r.AuthorName, &r.AuthorAvatar,
		&r.DestSpaceName, &r.DestSpaceCode,
		&r.CitySpaceName, &r.CitySpaceCode,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PostRow{}, pgx.ErrNoRows
		}

		return PostRow{}, fmt.Errorf("mutualaid: get post: %w", err)
	}

	return r, nil
}

// ListPosts returns a paginated, filtered list of active posts.
func (s *Store) ListPosts(ctx context.Context, f ListPostsFilter) ([]PostRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, "m.deleted_at IS NULL")
	conditions = append(conditions, "m.status = 'active'")
	conditions = append(conditions, "m.is_fulfilled = false")

	if f.PostType != "" {
		conditions = append(conditions, fmt.Sprintf("m.post_type = $%d", argIdx))
		args = append(args, f.PostType)
		argIdx++
	}

	if f.Category != "" {
		conditions = append(conditions, fmt.Sprintf("m.category = $%d", argIdx))
		args = append(args, f.Category)
		argIdx++
	}

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("m.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.CitySpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("m.city_space_id = $%d", argIdx))
		args = append(args, f.CitySpaceID)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	listQuery := fmt.Sprintf(`
SELECT m.id, m.user_id, m.post_type, m.category, m.title,
       m.description, m.destination_space_id, m.city_space_id, m.expires_at,
       m.is_fulfilled, m.status, m.created_at, m.updated_at, m.deleted_at,
       u.username AS author_name, up.avatar_url AS author_avatar,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       cs.name AS city_space_name, cs.geo_code AS city_space_code
FROM mutual_aid_posts m
LEFT JOIN users u ON u.id = m.user_id
LEFT JOIN user_profiles up ON up.user_id = m.user_id
LEFT JOIN spaces ds ON ds.id = m.destination_space_id
LEFT JOIN spaces cs ON cs.id = m.city_space_id
WHERE %s
ORDER BY m.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("mutualaid: list posts: %w", err)
	}
	defer rows.Close()

	var out []PostRow

	for rows.Next() {
		var r PostRow
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.UserID, &r.PostType, &r.Category, &r.Title,
			&r.Description, &r.DestinationSpaceID, &r.CitySpaceID, &r.ExpiresAt,
			&r.IsFulfilled, &r.Status, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
			&r.AuthorName, &r.AuthorAvatar,
			&r.DestSpaceName, &r.DestSpaceCode,
			&r.CitySpaceName, &r.CitySpaceCode,
		); err != nil {
			return nil, 0, fmt.Errorf("mutualaid: scan post row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("mutualaid: iterate post rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM mutual_aid_posts m
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("mutualaid: count posts: %w", err)
	}

	return out, total, nil
}

// UpdatePost updates the mutable fields of the author's own post. Only
// non-nil fields in input are applied.
func (s *Store) UpdatePost(ctx context.Context, id, tenantID pgtype.UUID, input UpdatePostInput) error {
	const q = `
UPDATE mutual_aid_posts
SET category = $2,
    title = $3,
    description = $4,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID, input.Category, input.Title, input.Description)
	if err != nil {
		return fmt.Errorf("mutualaid: update post: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// DeletePost soft-deletes a post (sets deleted_at = now()).
func (s *Store) DeletePost(ctx context.Context, id, tenantID pgtype.UUID) error {
	const q = `
UPDATE mutual_aid_posts
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID)
	if err != nil {
		return fmt.Errorf("mutualaid: delete post: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// MarkFulfilled sets is_fulfilled=true and status='fulfilled' on a post.
func (s *Store) MarkFulfilled(ctx context.Context, id, tenantID pgtype.UUID) error {
	const q = `
UPDATE mutual_aid_posts
SET is_fulfilled = true, status = 'fulfilled', updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID)
	if err != nil {
		return fmt.Errorf("mutualaid: mark fulfilled: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// RenewPost extends expires_at by 30 days from newExpiry. The post is
// flipped back to status='active' if it had expired, but is_fulfilled
// is NOT reset (BUG-015: renewing should not un-fulfill a post).
func (s *Store) RenewPost(ctx context.Context, id, tenantID pgtype.UUID, newExpiry pgtype.Timestamptz) error {
	const q = `
UPDATE mutual_aid_posts
SET expires_at = $2, status = 'active', updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID, newExpiry)
	if err != nil {
		return fmt.Errorf("mutualaid: renew post: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// AdminListPosts returns all posts (any status) for the moderation queue.
func (s *Store) AdminListPosts(ctx context.Context, f AdminListPostsFilter) ([]PostRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, "m.deleted_at IS NULL")

	if f.PostType != "" {
		conditions = append(conditions, fmt.Sprintf("m.post_type = $%d", argIdx))
		args = append(args, f.PostType)
		argIdx++
	}

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("m.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("m.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.CitySpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("m.city_space_id = $%d", argIdx))
		args = append(args, f.CitySpaceID)
		argIdx++
	}

	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("m.title ILIKE $%d", argIdx))
		args = append(args, "%"+f.Q+"%")
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

	listQuery := fmt.Sprintf(`
SELECT m.id, m.user_id, m.post_type, m.category, m.title,
       m.description, m.destination_space_id, m.city_space_id, m.expires_at,
       m.is_fulfilled, m.status, m.created_at, m.updated_at, m.deleted_at,
       u.username AS author_name, up.avatar_url AS author_avatar,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       cs.name AS city_space_name, cs.geo_code AS city_space_code
FROM mutual_aid_posts m
LEFT JOIN users u ON u.id = m.user_id
LEFT JOIN user_profiles up ON up.user_id = m.user_id
LEFT JOIN spaces ds ON ds.id = m.destination_space_id
LEFT JOIN spaces cs ON cs.id = m.city_space_id
WHERE %s
ORDER BY m.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("mutualaid: admin list posts: %w", err)
	}
	defer rows.Close()

	var out []PostRow

	for rows.Next() {
		var r PostRow
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.UserID, &r.PostType, &r.Category, &r.Title,
			&r.Description, &r.DestinationSpaceID, &r.CitySpaceID, &r.ExpiresAt,
			&r.IsFulfilled, &r.Status, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
			&r.AuthorName, &r.AuthorAvatar,
			&r.DestSpaceName, &r.DestSpaceCode,
			&r.CitySpaceName, &r.CitySpaceCode,
		); err != nil {
			return nil, 0, fmt.Errorf("mutualaid: admin scan post row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("mutualaid: admin iterate post rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM mutual_aid_posts m
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("mutualaid: admin count posts: %w", err)
	}

	return out, total, nil
}

// AdminUpdateStatus sets the status of a post (moderation action).
func (s *Store) AdminUpdateStatus(ctx context.Context, id, tenantID pgtype.UUID, status string) error {
	const q = `
UPDATE mutual_aid_posts
SET status = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID, status)
	if err != nil {
		return fmt.Errorf("mutualaid: admin update status: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ExpirePosts marks all active posts past their expires_at as 'expired'.
// Returns the number of posts expired. Called nightly by the
// mutual_aid:expire Asynq task (§85, ADR-093).
func (s *Store) ExpirePosts(ctx context.Context) (int64, error) {
	const q = `
UPDATE mutual_aid_posts
SET status = 'expired', updated_at = now()
WHERE status = 'active' AND expires_at < now() AND deleted_at IS NULL`

	cmd, err := s.pool.Writer.Exec(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("mutualaid: expire posts: %w", err)
	}

	return cmd.RowsAffected(), nil
}

// PostsExpiringSoon returns the IDs and owner IDs of active posts that
// will expire within the next 3 days. Used by the mutual_aid:expire task
// to send 3-day-prior notifications (§85).
func (s *Store) PostsExpiringSoon(ctx context.Context, withinHours int) ([]ExpiringPost, error) {
	const q = `
SELECT id, user_id, title, expires_at
FROM mutual_aid_posts
WHERE status = 'active'
  AND deleted_at IS NULL
  AND expires_at BETWEEN now() AND now() + ($1 || ' hours')::interval
ORDER BY expires_at ASC`

	rows, err := s.pool.Reader.Query(ctx, q, strconv.Itoa(withinHours))
	if err != nil {
		return nil, fmt.Errorf("mutualaid: expiring soon: %w", err)
	}

	defer rows.Close()

	var posts []ExpiringPost

	for rows.Next() {
		var p ExpiringPost
		if err := rows.Scan(&p.ID, &p.TenantID, &p.UserID, &p.Title, &p.ExpiresAt); err != nil {
			return nil, fmt.Errorf("mutualaid: expiring soon scan: %w", err)
		}

		posts = append(posts, p)
	}

	return posts, rows.Err()
}

// ExpiringPost is a post that is about to expire (for notifications).
type ExpiringPost struct {
	ID        pgtype.UUID
	TenantID  pgtype.UUID
	UserID    pgtype.UUID
	Title     string
	ExpiresAt pgtype.Timestamptz
}
