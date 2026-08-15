// Package achievement implements the Community Achievement Feed (§89, ADR-097).
//
// Achievements are records in the contents table with
// content_type = 'achievement', extended by the achievement_meta companion
// table for structured fields (milestone type, destination/origin space,
// anonymity flag). The package provides CRUD, filtered listing, a Space
// widget, and admin moderation — inheriting reactions, comments, bookmarks,
// space filing, and analytics from the Content Type System (§14).
//
// Ref: japaconnect-engineering/sections/89-community-achievement-feed.md
//
//	japaconnect-engineering/adr/ADR-097-community-achievement-feed.md
package achievement

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Store is the data-access layer for achievement_meta and achievement listing
// queries. It uses direct pgx queries because achievement_meta is a new table
// without sqlc-generated queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates an achievement store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// AchievementMeta holds the structured fields for a single achievement.
type AchievementMeta struct {
	ID                 pgtype.UUID
	TenantID           pgtype.UUID
	ContentID          pgtype.UUID
	MilestoneType      *string
	DestinationSpaceID pgtype.UUID
	OriginSpaceID      pgtype.UUID
	IsAnonymous        bool
	CreatedAt          pgtype.Timestamptz
}

// AchievementRow is the joined result of contents + achievement_meta + spaces
// + users, returned by list and get-by-slug queries.
type AchievementRow struct {
	ID              pgtype.UUID
	TenantID        pgtype.UUID
	AuthorID        pgtype.UUID
	Title           string
	Slug            string
	Body            string
	BodyHTML        string
	Status          string
	Visibility      string
	ViewCount       int32
	CommentCount    int32
	ReactionCount   int32
	PublishedAt     pgtype.Timestamptz
	CreatedAt       pgtype.Timestamptz
	UpdatedAt       pgtype.Timestamptz
	MilestoneType   *string
	OriginSpaceID   pgtype.UUID
	OriginSpaceName *string
	OriginSpaceCode *string
	DestSpaceID     pgtype.UUID
	DestSpaceName   *string
	DestSpaceCode   *string
	IsAnonymous     bool
	AuthorName      *string
	AuthorAvatar    *string
}

// GetSpaceLevel returns the level ('country', 'state', 'city', etc.) of the
// space identified by id, or pgx.ErrNoRows if not found. Used by the service
// to validate that destination_space_id is a country-level space (§89).
func (s *Store) GetSpaceLevel(ctx context.Context, id, tenantID pgtype.UUID) (string, error) {
	const q = `SELECT level FROM spaces WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`
	var level string
	if err := s.pool.Reader.QueryRow(ctx, q, id, tenantID).Scan(&level); err != nil {
		if err == pgx.ErrNoRows {
			return "", pgx.ErrNoRows
		}
		return "", fmt.Errorf("achievement: get space level: %w", err)
	}
	return level, nil
}

// CreateMeta inserts a new achievement_meta row linked to a contents row.
func (s *Store) CreateMeta(ctx context.Context, meta AchievementMeta) error {
	const q = `
INSERT INTO achievement_meta (
    tenant_id, content_id, milestone_type, destination_space_id,
    origin_space_id, is_anonymous
) VALUES ($1, $2, $3, $4, $5, $6)
`
	_, err := s.pool.Writer.Exec(ctx, q,
		meta.TenantID, meta.ContentID, meta.MilestoneType,
		meta.DestinationSpaceID, meta.OriginSpaceID, meta.IsAnonymous,
	)
	if err != nil {
		return fmt.Errorf("achievement: create achievement_meta: %w", err)
	}
	return nil
}

// GetMeta retrieves the achievement_meta row for a given content_id.
func (s *Store) GetMeta(ctx context.Context, contentID, tenantID pgtype.UUID) (AchievementMeta, error) {
	const q = `
SELECT id, tenant_id, content_id, milestone_type, destination_space_id,
       origin_space_id, is_anonymous, created_at
FROM achievement_meta
WHERE content_id = $1 AND tenant_id = $2
`
	var m AchievementMeta
	err := s.pool.Reader.QueryRow(ctx, q, contentID, tenantID).Scan(
		&m.ID, &m.TenantID, &m.ContentID, &m.MilestoneType,
		&m.DestinationSpaceID, &m.OriginSpaceID, &m.IsAnonymous, &m.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AchievementMeta{}, pgx.ErrNoRows
		}
		return AchievementMeta{}, fmt.Errorf("achievement: get achievement_meta: %w", err)
	}
	return m, nil
}

// UpdateMeta updates the mutable structured fields on an achievement_meta row.
// Called when the author edits their achievement.
func (s *Store) UpdateMeta(ctx context.Context, contentID, tenantID pgtype.UUID, meta AchievementMeta) error {
	const q = `
UPDATE achievement_meta
SET milestone_type = $3,
    destination_space_id = $4,
    origin_space_id = $5,
    is_anonymous = $6
WHERE content_id = $1 AND tenant_id = $2
`
	_, err := s.pool.Writer.Exec(ctx, q,
		contentID, tenantID,
		meta.MilestoneType, meta.DestinationSpaceID, meta.OriginSpaceID,
		meta.IsAnonymous,
	)
	if err != nil {
		return fmt.Errorf("achievement: update achievement_meta: %w", err)
	}
	return nil
}

// ListAchievementsFilter holds the filter parameters for listing achievements.
type ListAchievementsFilter struct {
	TenantID      pgtype.UUID
	DestSpaceID   pgtype.UUID // optional
	MilestoneType string      // optional
	Sort          string      // "top" | "latest" (default: latest)
	Status        string      // filter by status (default: published)
	Limit         int32
	Offset        int32
}

// ListAchievements returns a paginated, filtered list of published
// achievements. The query joins contents + achievement_meta + spaces
// (origin + destination) + users (author name/avatar). Sort "top" orders by
// view_count + reaction count; "latest" orders by published_at DESC.
func (s *Store) ListAchievements(ctx context.Context, f ListAchievementsFilter) ([]AchievementRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("c.tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	conditions = append(conditions, fmt.Sprintf("c.content_type = $%d", argIdx))
	args = append(args, "achievement")
	argIdx++

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("c.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	} else {
		conditions = append(conditions, "c.status = 'published'")
	}

	conditions = append(conditions, "c.deleted_at IS NULL")

	if f.DestSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("am.destination_space_id = $%d", argIdx))
		args = append(args, f.DestSpaceID)
		argIdx++
	}
	if f.MilestoneType != "" {
		conditions = append(conditions, fmt.Sprintf("am.milestone_type = $%d", argIdx))
		args = append(args, f.MilestoneType)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	orderClause := "c.published_at DESC NULLS LAST, c.created_at DESC"
	if f.Sort == "top" {
		orderClause = "c.view_count DESC, c.comment_count DESC, c.published_at DESC"
	}

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
SELECT c.id, c.tenant_id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       (SELECT COUNT(*) FROM reactions r
        WHERE r.entity_id = c.id AND r.tenant_id = c.tenant_id
          AND r.entity_type = 'content') AS reaction_count,
       c.published_at, c.created_at, c.updated_at,
       am.milestone_type,
       am.origin_space_id, os.name AS origin_space_name, os.geo_code AS origin_space_code,
       am.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       am.is_anonymous,
       u.username AS author_name, up.avatar_url AS author_avatar
FROM contents c
JOIN achievement_meta am ON am.content_id = c.id AND am.tenant_id = c.tenant_id
LEFT JOIN spaces os ON os.id = am.origin_space_id
JOIN spaces ds ON ds.id = am.destination_space_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
WHERE %s
ORDER BY %s
LIMIT $%d OFFSET $%d
`, whereClause, orderClause, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("achievement: list achievements: %w", err)
	}
	defer rows.Close()

	var out []AchievementRow
	for rows.Next() {
		var r AchievementRow
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.AuthorID, &r.Title, &r.Slug, &r.Body, &r.BodyHTML,
			&r.Status, &r.Visibility, &r.ViewCount, &r.CommentCount, &r.ReactionCount,
			&r.PublishedAt, &r.CreatedAt, &r.UpdatedAt,
			&r.MilestoneType,
			&r.OriginSpaceID, &r.OriginSpaceName, &r.OriginSpaceCode,
			&r.DestSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
			&r.IsAnonymous,
			&r.AuthorName, &r.AuthorAvatar,
		); err != nil {
			return nil, 0, fmt.Errorf("achievement: scan achievement row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("achievement: iterate achievement rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM contents c
JOIN achievement_meta am ON am.content_id = c.id AND am.tenant_id = c.tenant_id
WHERE %s
`, whereClause)
	// Rebuild count args without limit/offset (same conditions, fewer params).
	countArgs := args[:len(args)-2]
	var total int64
	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("achievement: count achievements: %w", err)
	}

	return out, total, nil
}

// achievementSelectColumns is the shared column list + joins for the
// achievement detail query, used by both GetBySlug and GetByID. Includes a
// correlated reaction_count subquery (§29 polymorphic reactions, entity_type
// 'content') so every detail response surfaces the true reaction count.
const achievementSelectColumns = `
SELECT c.id, c.tenant_id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       (SELECT COUNT(*) FROM reactions r
        WHERE r.entity_id = c.id AND r.tenant_id = c.tenant_id
          AND r.entity_type = 'content') AS reaction_count,
       c.published_at, c.created_at, c.updated_at,
       am.milestone_type,
       am.origin_space_id, os.name AS origin_space_name, os.geo_code AS origin_space_code,
       am.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       am.is_anonymous,
       u.username AS author_name, up.avatar_url AS author_avatar
FROM contents c
JOIN achievement_meta am ON am.content_id = c.id AND am.tenant_id = c.tenant_id
LEFT JOIN spaces os ON os.id = am.origin_space_id
JOIN spaces ds ON ds.id = am.destination_space_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
`

// scanAchievementRow scans a single achievement detail row into an AchievementRow.
func scanAchievementRow(row pgx.Row) (AchievementRow, error) {
	var r AchievementRow
	if err := row.Scan(
		&r.ID, &r.TenantID, &r.AuthorID, &r.Title, &r.Slug, &r.Body, &r.BodyHTML,
		&r.Status, &r.Visibility, &r.ViewCount, &r.CommentCount, &r.ReactionCount,
		&r.PublishedAt, &r.CreatedAt, &r.UpdatedAt,
		&r.MilestoneType,
		&r.OriginSpaceID, &r.OriginSpaceName, &r.OriginSpaceCode,
		&r.DestSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
		&r.IsAnonymous,
		&r.AuthorName, &r.AuthorAvatar,
	); err != nil {
		if err == pgx.ErrNoRows {
			return AchievementRow{}, pgx.ErrNoRows
		}
		return AchievementRow{}, err
	}
	return r, nil
}

// GetBySlug returns a single achievement by its slug. Only published
// achievements are visible to the public; an unpublished (e.g. admin-
// moderated) achievement is only visible to its own author. Pass a zero
// (invalid) viewerID for anonymous access. Joins contents + achievement_meta
// + spaces + users.
func (s *Store) GetBySlug(ctx context.Context, slug string, tenantID, viewerID pgtype.UUID) (AchievementRow, error) {
	q := achievementSelectColumns + `
WHERE c.tenant_id = $1 AND c.slug = $2 AND c.content_type = 'achievement'
  AND c.deleted_at IS NULL
  AND (c.status = 'published'`
	args := []any{tenantID, slug}
	if viewerID.Valid {
		q += ` OR c.author_id = $3`
		args = append(args, viewerID)
	}
	q += ")\nLIMIT 1\n"

	r, err := scanAchievementRow(s.pool.Reader.QueryRow(ctx, q, args...))
	if err != nil {
		return AchievementRow{}, fmt.Errorf("achievement: get by slug: %w", err)
	}
	return r, nil
}

// GetByID returns a single achievement by its content id, with the same
// published-or-author visibility rule as GetBySlug. Used by the
// submit/edit response paths so every mutation returns the complete DTO
// (reaction_count, space names, author info) instead of the partial
// metaToDTO shape.
func (s *Store) GetByID(ctx context.Context, contentID, tenantID, viewerID pgtype.UUID) (AchievementRow, error) {
	q := achievementSelectColumns + `
WHERE c.tenant_id = $1 AND c.id = $2 AND c.content_type = 'achievement'
  AND c.deleted_at IS NULL
  AND (c.status = 'published'`
	args := []any{tenantID, contentID}
	if viewerID.Valid {
		q += ` OR c.author_id = $3`
		args = append(args, viewerID)
	}
	q += ")\nLIMIT 1\n"

	r, err := scanAchievementRow(s.pool.Reader.QueryRow(ctx, q, args...))
	if err != nil {
		return AchievementRow{}, fmt.Errorf("achievement: get by id: %w", err)
	}
	return r, nil
}

// GetByIDAnyStatus returns a single achievement by its content id regardless
// of publication status. For admin/moderation response paths only — the
// caller must hold the achievement.moderate permission (enforced at the
// route level).
func (s *Store) GetByIDAnyStatus(ctx context.Context, contentID, tenantID pgtype.UUID) (AchievementRow, error) {
	q := achievementSelectColumns + `
WHERE c.tenant_id = $1 AND c.id = $2 AND c.content_type = 'achievement'
  AND c.deleted_at IS NULL
LIMIT 1
`
	r, err := scanAchievementRow(s.pool.Reader.QueryRow(ctx, q, tenantID, contentID))
	if err != nil {
		return AchievementRow{}, fmt.Errorf("achievement: get by id (admin): %w", err)
	}
	return r, nil
}

// ListBySpace returns published achievements filed in the Space identified by
// its slug, for the Space widget. Ordered by published_at DESC.
func (s *Store) ListBySpace(ctx context.Context, spaceSlug string, tenantID pgtype.UUID, limit int32) ([]AchievementRow, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	const q = `
SELECT c.id, c.tenant_id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       (SELECT COUNT(*) FROM reactions r
        WHERE r.entity_id = c.id AND r.tenant_id = c.tenant_id
          AND r.entity_type = 'content') AS reaction_count,
       c.published_at, c.created_at, c.updated_at,
       am.milestone_type,
       am.origin_space_id, os.name AS origin_space_name, os.geo_code AS origin_space_code,
       am.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       am.is_anonymous,
       u.username AS author_name, up.avatar_url AS author_avatar
FROM contents c
JOIN achievement_meta am ON am.content_id = c.id AND am.tenant_id = c.tenant_id
LEFT JOIN spaces os ON os.id = am.origin_space_id
JOIN spaces ds ON ds.id = am.destination_space_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
WHERE c.tenant_id = $1 AND c.content_type = 'achievement'
  AND c.status = 'published' AND c.deleted_at IS NULL
  AND ds.slug = $2
ORDER BY c.published_at DESC NULLS LAST, c.created_at DESC
LIMIT $3
`
	rows, err := s.pool.Reader.Query(ctx, q, tenantID, spaceSlug, limit)
	if err != nil {
		return nil, fmt.Errorf("achievement: list by space: %w", err)
	}
	defer rows.Close()

	var out []AchievementRow
	for rows.Next() {
		var r AchievementRow
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.AuthorID, &r.Title, &r.Slug, &r.Body, &r.BodyHTML,
			&r.Status, &r.Visibility, &r.ViewCount, &r.CommentCount, &r.ReactionCount,
			&r.PublishedAt, &r.CreatedAt, &r.UpdatedAt,
			&r.MilestoneType,
			&r.OriginSpaceID, &r.OriginSpaceName, &r.OriginSpaceCode,
			&r.DestSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
			&r.IsAnonymous,
			&r.AuthorName, &r.AuthorAvatar,
		); err != nil {
			return nil, fmt.Errorf("achievement: list by space scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("achievement: list by space iterate: %w", err)
	}
	return out, nil
}

// AdminListFilter holds filter parameters for the admin moderation list.
type AdminListFilter struct {
	TenantID      pgtype.UUID
	Status        string // optional: filter by content status
	MilestoneType string // optional: filter by milestone type
	Q             string // optional: search title
	Limit         int32
	Offset        int32
}

// AdminList returns all achievements (any status) for the moderation queue.
func (s *Store) AdminList(ctx context.Context, f AdminListFilter) ([]AchievementRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("c.tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	conditions = append(conditions, fmt.Sprintf("c.content_type = $%d", argIdx))
	args = append(args, "achievement")
	argIdx++

	conditions = append(conditions, "c.deleted_at IS NULL")

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("c.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}
	if f.MilestoneType != "" {
		conditions = append(conditions, fmt.Sprintf("am.milestone_type = $%d", argIdx))
		args = append(args, f.MilestoneType)
		argIdx++
	}
	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("c.title ILIKE $%d", argIdx))
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
SELECT c.id, c.tenant_id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       (SELECT COUNT(*) FROM reactions r
        WHERE r.entity_id = c.id AND r.tenant_id = c.tenant_id
          AND r.entity_type = 'content') AS reaction_count,
       c.published_at, c.created_at, c.updated_at,
       am.milestone_type,
       am.origin_space_id, os.name AS origin_space_name, os.geo_code AS origin_space_code,
       am.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       am.is_anonymous,
       u.username AS author_name, up.avatar_url AS author_avatar
FROM contents c
JOIN achievement_meta am ON am.content_id = c.id AND am.tenant_id = c.tenant_id
LEFT JOIN spaces os ON os.id = am.origin_space_id
JOIN spaces ds ON ds.id = am.destination_space_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
WHERE %s
ORDER BY c.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("achievement: admin list: %w", err)
	}
	defer rows.Close()

	var out []AchievementRow
	for rows.Next() {
		var r AchievementRow
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.AuthorID, &r.Title, &r.Slug, &r.Body, &r.BodyHTML,
			&r.Status, &r.Visibility, &r.ViewCount, &r.CommentCount, &r.ReactionCount,
			&r.PublishedAt, &r.CreatedAt, &r.UpdatedAt,
			&r.MilestoneType,
			&r.OriginSpaceID, &r.OriginSpaceName, &r.OriginSpaceCode,
			&r.DestSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
			&r.IsAnonymous,
			&r.AuthorName, &r.AuthorAvatar,
		); err != nil {
			return nil, 0, fmt.Errorf("achievement: admin scan achievement row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("achievement: admin iterate achievement rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM contents c
JOIN achievement_meta am ON am.content_id = c.id AND am.tenant_id = c.tenant_id
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]
	var total int64
	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("achievement: admin count achievements: %w", err)
	}

	return out, total, nil
}
