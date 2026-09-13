// Package story implements the Community Story Wall (§79, ADR-087).
//
// Stories are records in the contents table with content_type = 'story',
// extended by the story_meta companion table for structured fields (origin
// country, destination country, visa category, profession, relocation year,
// challenge/advice fields). The package provides CRUD, filtered listing,
// and admin verification — inheriting reactions, comments, bookmarks, space
// filing, and analytics from the Content Type System (§14).
//
// Ref: japaconnect-engineering/sections/79-community-story-wall.md
//
//	japaconnect-engineering/adr/ADR-087-community-story-wall.md
package story

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Store is the data-access layer for story_meta and story listing queries.
// It uses direct pgx queries because story_meta is a new table without
// sqlc-generated queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a story store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// StoryMeta holds the structured fields for a single story.
//
//nolint:revive // exported: stutter naming is intentional
type StoryMeta struct {
	ID                 pgtype.UUID
	TenantID           pgtype.UUID
	ContentID          pgtype.UUID
	OriginSpaceID      pgtype.UUID
	DestinationSpaceID pgtype.UUID
	RelocationYear     pgtype.Int2
	VisaCategory       *string
	Profession         *string
	BiggestChallenge   *string
	MostUsefulResource *string
	BestAdvice         *string
	Verified           bool
	AiGenerated        bool
	AiModel            *string
	HitlReviewedBy     pgtype.UUID
	AiGeneratedAt      pgtype.Timestamptz
	HitlReviewedAt     pgtype.Timestamptz
	IsJapabotIndexed   bool
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
}

// StoryRow is the joined result of contents + story_meta + space names,
// returned by list and get-by-slug queries.
//
//nolint:revive // exported: stutter naming is intentional
type StoryRow struct {
	ID                 pgtype.UUID
	TenantID           pgtype.UUID
	AuthorID           pgtype.UUID
	Title              string
	Slug               string
	Body               string
	BodyHTML           string
	Status             string
	Visibility         string
	ViewCount          int32
	CommentCount       int32
	PublishedAt        pgtype.Timestamptz
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
	OriginSpaceID      pgtype.UUID
	OriginSpaceName    *string
	OriginSpaceCode    *string
	DestSpaceID        pgtype.UUID
	DestSpaceName      *string
	DestSpaceCode      *string
	RelocationYear     pgtype.Int2
	VisaCategory       *string
	Profession         *string
	BiggestChallenge   *string
	MostUsefulResource *string
	BestAdvice         *string
	Verified           bool
	AuthorName         *string
	AuthorUsername     *string
	AuthorDisplayName  *string
	AuthorAvatar       *string
	// Series membership — stories may belong to a blog_series like any other
	// content type (series nav on the single page mirrors the blog layout).
	SeriesID    pgtype.UUID
	SeriesTitle *string
	SeriesSlug  *string
	SeriesPart  pgtype.Int4
	SeriesTotal int64
	// §34 cover media — resolved into signed URLs by the service layer.
	FeaturedImageID       pgtype.UUID
	FeaturedStorageBucket *string
	FeaturedStoragePath   *string
	HeaderImageID         pgtype.UUID
	HeaderStorageBucket   *string
	HeaderStoragePath     *string
}

// CreateStoryMeta inserts a new story_meta row linked to a contents row.
// Upserts on the (content_id, tenant_id) unique key — the generic contents
// pipeline's syncStoryMeta may already have created the row (e.g. a story
// created through /create before POST /stories completes its own insert).
func (s *Store) CreateStoryMeta(ctx context.Context, meta StoryMeta) error {
	const q = `
INSERT INTO story_meta (
    content_id, origin_space_id, destination_space_id,
    relocation_year, visa_category, profession, biggest_challenge,
    most_useful_resource, best_advice, verified,
    ai_generated, ai_model, hitl_reviewed_by, ai_generated_at,
    hitl_reviewed_at, is_japabot_indexed
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (content_id) DO UPDATE SET
    origin_space_id = EXCLUDED.origin_space_id,
    destination_space_id = EXCLUDED.destination_space_id,
    relocation_year = EXCLUDED.relocation_year,
    visa_category = EXCLUDED.visa_category,
    profession = EXCLUDED.profession,
    biggest_challenge = EXCLUDED.biggest_challenge,
    most_useful_resource = EXCLUDED.most_useful_resource,
    best_advice = EXCLUDED.best_advice,
    verified = EXCLUDED.verified,
    ai_generated = EXCLUDED.ai_generated,
    ai_model = EXCLUDED.ai_model,
    hitl_reviewed_by = EXCLUDED.hitl_reviewed_by,
    ai_generated_at = EXCLUDED.ai_generated_at,
    hitl_reviewed_at = EXCLUDED.hitl_reviewed_at,
    is_japabot_indexed = EXCLUDED.is_japabot_indexed,
    updated_at = now()
`

	_, err := s.pool.Writer.Exec(
		ctx, q,
		meta.TenantID, meta.ContentID, meta.OriginSpaceID, meta.DestinationSpaceID,
		meta.RelocationYear, meta.VisaCategory, meta.Profession, meta.BiggestChallenge,
		meta.MostUsefulResource, meta.BestAdvice, meta.Verified,
		meta.AiGenerated, meta.AiModel, meta.HitlReviewedBy, meta.AiGeneratedAt,
		meta.HitlReviewedAt, meta.IsJapabotIndexed,
	)
	if err != nil {
		return fmt.Errorf("story: create story_meta: %w", err)
	}

	return nil
}

// UpdateStoryMeta updates the mutable structured fields on a story_meta row.
// Called when a story is edited. Edits strip verification (verified=false and
// the ADR-088 HITL review trail is cleared) until an admin re-verifies — §79.
func (s *Store) UpdateStoryMeta(ctx context.Context, contentID, tenantID pgtype.UUID, meta StoryMeta) error {
	const q = `
UPDATE story_meta
SET origin_space_id = $2,
    destination_space_id = $3,
    relocation_year = $4,
    visa_category = $5,
    profession = $6,
    biggest_challenge = $7,
    most_useful_resource = $8,
    best_advice = $9,
    verified = false,
    hitl_reviewed_by = NULL,
    hitl_reviewed_at = NULL,
    updated_at = now()
WHERE content_id = $1
`

	_, err := s.pool.Writer.Exec(
		ctx, q,
		contentID, tenantID,
		meta.OriginSpaceID, meta.DestinationSpaceID,
		meta.RelocationYear, meta.VisaCategory, meta.Profession,
		meta.BiggestChallenge, meta.MostUsefulResource, meta.BestAdvice,
	)
	if err != nil {
		return fmt.Errorf("story: update story_meta: %w", err)
	}

	return nil
}

// GetCountrySpaceGeoCode returns the geo_code of a country-level space —
// §79 origin/destination pickers only accept country spaces, so a space at
// any other level returns pgx.ErrNoRows (surfaced as a 400). The code keeps
// the content row's source_country/target_country in sync when a story's
// origin or destination changes through POST/PATCH /stories.
func (s *Store) GetCountrySpaceGeoCode(ctx context.Context, spaceID, tenantID pgtype.UUID) (string, error) {
	const q = `SELECT geo_code FROM spaces WHERE id = $1 AND level = 'country' AND deleted_at IS NULL`

	var code *string
	err := s.pool.Reader.QueryRow(ctx, q, spaceID, tenantID).Scan(&code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", pgx.ErrNoRows
		}

		return "", fmt.Errorf("story: get space geo code: %w", err)
	}

	if code == nil {
		return "", nil
	}

	return *code, nil
}

// PruneContentSpacesExcept removes content_spaces rows that do not match the
// given space. Stories file in exactly one space — their destination country
// (§79) — so on a destination change the previous filing must not linger as
// an "additional" space (the generic UpdateContentSpaces preserves additional
// rows, which is correct for multi-space content but not for stories).
func (s *Store) PruneContentSpacesExcept(ctx context.Context, contentID, keepSpaceID, tenantID pgtype.UUID) error {
	// content_spaces has no tenant_id column — the tenant scope is enforced
	// by joining through contents.
	const q = `DELETE FROM content_spaces
WHERE content_id = $1 AND space_id <> $2
  AND content_id IN (SELECT id FROM contents WHERE id = $1)`

	if _, err := s.pool.Writer.Exec(ctx, q, contentID, keepSpaceID, tenantID); err != nil {
		return fmt.Errorf("story: prune content_spaces: %w", err)
	}

	return nil
}

// GetStoryMeta retrieves the story_meta row for a given content_id.
func (s *Store) GetStoryMeta(ctx context.Context, contentID, tenantID pgtype.UUID) (StoryMeta, error) {
	const q = `
SELECT id, content_id, origin_space_id, destination_space_id,
       relocation_year, visa_category, profession, biggest_challenge,
       most_useful_resource, best_advice, verified,
       ai_generated, ai_model, hitl_reviewed_by, ai_generated_at,
       hitl_reviewed_at, is_japabot_indexed, created_at, updated_at
FROM story_meta
WHERE content_id = $1
`

	var m StoryMeta

	err := s.pool.Reader.QueryRow(ctx, q, contentID, tenantID).Scan(
		&m.ID, &m.ContentID, &m.OriginSpaceID, &m.DestinationSpaceID,
		&m.RelocationYear, &m.VisaCategory, &m.Profession, &m.BiggestChallenge,
		&m.MostUsefulResource, &m.BestAdvice, &m.Verified,
		&m.AiGenerated, &m.AiModel, &m.HitlReviewedBy, &m.AiGeneratedAt,
		&m.HitlReviewedAt, &m.IsJapabotIndexed, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StoryMeta{}, pgx.ErrNoRows
		}

		return StoryMeta{}, fmt.Errorf("story: get story_meta: %w", err)
	}

	return m, nil
}

// VerifyStory marks a story as verified by an admin. Sets hitl_reviewed_by
// and hitl_reviewed_at per ADR-088 HITL review audit trail.
func (s *Store) VerifyStory(ctx context.Context, contentID, tenantID, adminUserID pgtype.UUID) error {
	const q = `
UPDATE story_meta
SET verified = true,
    hitl_reviewed_by = $2,
    hitl_reviewed_at = now(),
    updated_at = now()
WHERE content_id = $1
`

	cmd, err := s.pool.Writer.Exec(ctx, q, contentID, tenantID, adminUserID)
	if err != nil {
		return fmt.Errorf("story: verify story: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// UnverifyStory marks a story as unverified by an admin (story.verify
// permission). Clears the verified flag and the HITL review audit trail.
func (s *Store) UnverifyStory(ctx context.Context, contentID, tenantID pgtype.UUID) error {
	const q = `
UPDATE story_meta
SET verified = false,
    hitl_reviewed_by = null,
    hitl_reviewed_at = null,
    updated_at = now()
WHERE content_id = $1
`

	cmd, err := s.pool.Writer.Exec(ctx, q, contentID, tenantID)
	if err != nil {
		return fmt.Errorf("story: unverify story: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ListStoriesFilter holds the filter parameters for listing stories.
type ListStoriesFilter struct {
	TenantID      pgtype.UUID
	OriginSpaceID pgtype.UUID // optional
	DestSpaceID   pgtype.UUID // optional
	VisaCategory  string      // optional
	Profession    string      // optional
	Sort          string      // "top" | "latest" (default: latest)
	Status        string      // filter by status (default: published)
	// Visibility gating (§14): zero RequesterID = guest — only public stories.
	// Members-visible stories require CanSeeMembers; private ones require the
	// requester to be the author or a moderator (CanSeePrivate).
	RequesterID   pgtype.UUID
	CanSeeMembers bool
	CanSeePrivate bool
	Limit         int32
	Offset        int32
}

// ListStories returns a paginated, filtered list of published stories.
// The query joins contents + story_meta + spaces (origin + destination) +
// users (author name/avatar). Sort "top" orders by view_count + comment_count;
// "latest" orders by published_at DESC.
func (s *Store) ListStories(ctx context.Context, f ListStoriesFilter) ([]StoryRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("c.content_type = $%d", argIdx))
	args = append(args, "story")
	argIdx++

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("c.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	} else {
		conditions = append(conditions, "c.status = 'published'")
	}

	conditions = append(conditions, "c.deleted_at IS NULL")

	if f.OriginSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("sm.origin_space_id = $%d", argIdx))
		args = append(args, f.OriginSpaceID)
		argIdx++
	}

	if f.DestSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("sm.destination_space_id = $%d", argIdx))
		args = append(args, f.DestSpaceID)
		argIdx++
	}

	if f.VisaCategory != "" {
		conditions = append(conditions, fmt.Sprintf("sm.visa_category = $%d", argIdx))
		args = append(args, f.VisaCategory)
		argIdx++
	}

	if f.Profession != "" {
		conditions = append(conditions, fmt.Sprintf("sm.profession ILIKE $%d", argIdx))
		args = append(args, "%"+f.Profession+"%")
		argIdx++
	}

	// Visibility gating (§14) — same clause shape as internal/db/queries/
	// contents.sql: public for everyone, members for authenticated callers,
	// private for the author or a moderator, plus the author always sees
	// their own row.
	conditions = append(conditions, fmt.Sprintf(`(
		c.visibility = 'public'
		OR (c.author_id = $%d AND c.author_id IS NOT NULL)
		OR (c.visibility = 'members' AND $%d)
		OR (c.visibility = 'private' AND $%d)
	)`, argIdx, argIdx+1, argIdx+2))
	args = append(args, f.RequesterID, f.CanSeeMembers, f.CanSeePrivate)
	argIdx += 3

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
SELECT c.id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       c.published_at, c.created_at, c.updated_at,
       sm.origin_space_id, os.name AS origin_space_name, os.geo_code AS origin_space_code,
       sm.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       sm.relocation_year, sm.visa_category, sm.profession,
       sm.biggest_challenge, sm.most_useful_resource, sm.best_advice,
       sm.verified,
       u.username AS author_name, u.username AS author_username, up.display_name AS author_display_name, up.avatar_url AS author_avatar
FROM contents c
JOIN story_meta sm ON sm.content_id = c.id
LEFT JOIN spaces os ON os.id = sm.origin_space_id
JOIN spaces ds ON ds.id = sm.destination_space_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
WHERE %s
ORDER BY %s
LIMIT $%d OFFSET $%d
`, whereClause, orderClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("story: list stories: %w", err)
	}
	defer rows.Close()

	var out []StoryRow

	for rows.Next() {
		var r StoryRow
		if err := rows.Scan(
			&r.ID, &r.AuthorID, &r.Title, &r.Slug, &r.Body, &r.BodyHTML,
			&r.Status, &r.Visibility, &r.ViewCount, &r.CommentCount,
			&r.PublishedAt, &r.CreatedAt, &r.UpdatedAt,
			&r.OriginSpaceID, &r.OriginSpaceName, &r.OriginSpaceCode,
			&r.DestSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
			&r.RelocationYear, &r.VisaCategory, &r.Profession,
			&r.BiggestChallenge, &r.MostUsefulResource, &r.BestAdvice,
			&r.Verified,
			&r.AuthorName, &r.AuthorUsername, &r.AuthorDisplayName, &r.AuthorAvatar,
		); err != nil {
			return nil, 0, fmt.Errorf("story: scan story row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("story: iterate story rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM contents c
JOIN story_meta sm ON sm.content_id = c.id
WHERE %s
`, whereClause)
	// Rebuild count args without limit/offset (same conditions, fewer params).
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("story: count stories: %w", err)
	}

	return out, total, nil
}

// GetStoryBySlug returns a single story by its slug (any status), joining
// contents + story_meta + spaces + users. Status and visibility gating are
// enforced by the caller (HandleGetBySlug): published stories are gated by
// §14 visibility; non-published stories are author- or moderator-only (§79).
func (s *Store) GetStoryBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (StoryRow, error) {
	const q = `
SELECT c.id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       c.published_at, c.created_at, c.updated_at,
       sm.origin_space_id, os.name AS origin_space_name, os.geo_code AS origin_space_code,
       sm.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       sm.relocation_year, sm.visa_category, sm.profession,
       sm.biggest_challenge, sm.most_useful_resource, sm.best_advice,
       sm.verified,
       u.username AS author_name, u.username AS author_username, up.display_name AS author_display_name, up.avatar_url AS author_avatar,
       bs.id AS series_id, bs.title AS series_title, bs.slug AS series_slug,
       bsp.position AS series_part,
       (SELECT COUNT(*) FROM blog_series_posts WHERE series_id = bs.id) AS series_total,
       c.featured_image_id, fm.storage_bucket AS featured_storage_bucket, fm.storage_path AS featured_storage_path,
       c.header_image_id, hm.storage_bucket AS header_storage_bucket, hm.storage_path AS header_storage_path
FROM contents c
JOIN story_meta sm ON sm.content_id = c.id
LEFT JOIN spaces os ON os.id = sm.origin_space_id
JOIN spaces ds ON ds.id = sm.destination_space_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN blog_series_posts bsp ON bsp.content_id = c.id
LEFT JOIN blog_series bs ON bs.id = bsp.series_id AND bs.deleted_at IS NULL
LEFT JOIN media fm ON fm.id = c.featured_image_id AND fm.deleted_at IS NULL
LEFT JOIN media hm ON hm.id = c.header_image_id AND hm.deleted_at IS NULL
WHERE c.slug = $1 AND c.content_type = 'story'
  AND c.deleted_at IS NULL
LIMIT 1
`

	var r StoryRow

	err := s.pool.Reader.QueryRow(ctx, q, tenantID, slug).Scan(
		&r.ID, &r.AuthorID, &r.Title, &r.Slug, &r.Body, &r.BodyHTML,
		&r.Status, &r.Visibility, &r.ViewCount, &r.CommentCount,
		&r.PublishedAt, &r.CreatedAt, &r.UpdatedAt,
		&r.OriginSpaceID, &r.OriginSpaceName, &r.OriginSpaceCode,
		&r.DestSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
		&r.RelocationYear, &r.VisaCategory, &r.Profession,
		&r.BiggestChallenge, &r.MostUsefulResource, &r.BestAdvice,
		&r.Verified,
		&r.AuthorName, &r.AuthorUsername, &r.AuthorDisplayName, &r.AuthorAvatar,
		&r.SeriesID, &r.SeriesTitle, &r.SeriesSlug, &r.SeriesPart, &r.SeriesTotal,
		&r.FeaturedImageID, &r.FeaturedStorageBucket, &r.FeaturedStoragePath,
		&r.HeaderImageID, &r.HeaderStorageBucket, &r.HeaderStoragePath,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StoryRow{}, pgx.ErrNoRows
		}

		return StoryRow{}, fmt.Errorf("story: get by slug: %w", err)
	}

	return r, nil
}

// GetStoryByID returns a single story by its content_id (any status), joining
// contents + story_meta + spaces + users. Used by Submit/EditOwn to return a
// fully-populated DTO with origin/destination space names and author info.
func (s *Store) GetStoryByID(ctx context.Context, contentID, tenantID pgtype.UUID) (StoryRow, error) {
	const q = `
SELECT c.id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       c.published_at, c.created_at, c.updated_at,
       sm.origin_space_id, os.name AS origin_space_name, os.geo_code AS origin_space_code,
       sm.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       sm.relocation_year, sm.visa_category, sm.profession,
       sm.biggest_challenge, sm.most_useful_resource, sm.best_advice,
       sm.verified,
       u.username AS author_name, u.username AS author_username, up.display_name AS author_display_name, up.avatar_url AS author_avatar,
       bs.id AS series_id, bs.title AS series_title, bs.slug AS series_slug,
       bsp.position AS series_part,
       (SELECT COUNT(*) FROM blog_series_posts WHERE series_id = bs.id) AS series_total,
       c.featured_image_id, fm.storage_bucket AS featured_storage_bucket, fm.storage_path AS featured_storage_path,
       c.header_image_id, hm.storage_bucket AS header_storage_bucket, hm.storage_path AS header_storage_path
FROM contents c
JOIN story_meta sm ON sm.content_id = c.id
LEFT JOIN spaces os ON os.id = sm.origin_space_id
JOIN spaces ds ON ds.id = sm.destination_space_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN blog_series_posts bsp ON bsp.content_id = c.id
LEFT JOIN blog_series bs ON bs.id = bsp.series_id AND bs.deleted_at IS NULL
LEFT JOIN media fm ON fm.id = c.featured_image_id AND fm.deleted_at IS NULL
LEFT JOIN media hm ON hm.id = c.header_image_id AND hm.deleted_at IS NULL
WHERE c.id = $1 AND c.content_type = 'story'
  AND c.deleted_at IS NULL
LIMIT 1
`

	var r StoryRow

	err := s.pool.Reader.QueryRow(ctx, q, tenantID, contentID).Scan(
		&r.ID, &r.AuthorID, &r.Title, &r.Slug, &r.Body, &r.BodyHTML,
		&r.Status, &r.Visibility, &r.ViewCount, &r.CommentCount,
		&r.PublishedAt, &r.CreatedAt, &r.UpdatedAt,
		&r.OriginSpaceID, &r.OriginSpaceName, &r.OriginSpaceCode,
		&r.DestSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
		&r.RelocationYear, &r.VisaCategory, &r.Profession,
		&r.BiggestChallenge, &r.MostUsefulResource, &r.BestAdvice,
		&r.Verified,
		&r.AuthorName, &r.AuthorUsername, &r.AuthorDisplayName, &r.AuthorAvatar,
		&r.SeriesID, &r.SeriesTitle, &r.SeriesSlug, &r.SeriesPart, &r.SeriesTotal,
		&r.FeaturedImageID, &r.FeaturedStorageBucket, &r.FeaturedStoragePath,
		&r.HeaderImageID, &r.HeaderStorageBucket, &r.HeaderStoragePath,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StoryRow{}, pgx.ErrNoRows
		}

		return StoryRow{}, fmt.Errorf("story: get by id: %w", err)
	}

	return r, nil
}

// AdminListStoriesFilter holds filter parameters for the admin moderation list.
type AdminListStoriesFilter struct {
	TenantID    pgtype.UUID
	Status      string      // optional: filter by content status
	Verified    *bool       // optional: filter by verified state
	Q           string      // optional: search title
	Sort        string      // "latest" (default), "oldest", "title"
	DestSpaceID pgtype.UUID // optional: filter by destination country
	Limit       int32
	Offset      int32
}

// AdminListStories returns all stories (any status) for the moderation queue.
func (s *Store) AdminListStories(ctx context.Context, f AdminListStoriesFilter) ([]StoryRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("c.content_type = $%d", argIdx))
	args = append(args, "story")
	argIdx++

	conditions = append(conditions, "c.deleted_at IS NULL")

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("c.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}

	if f.Verified != nil {
		conditions = append(conditions, fmt.Sprintf("sm.verified = $%d", argIdx))
		args = append(args, *f.Verified)
		argIdx++
	}

	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("c.title ILIKE $%d", argIdx))
		args = append(args, "%"+f.Q+"%")
		argIdx++
	}

	if f.DestSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("sm.destination_space_id = $%d", argIdx))
		args = append(args, f.DestSpaceID)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Sort clause — admin list must support sort per the geo-aware tool pattern.
	// "oldest" → created_at ASC; "title" → title ASC; default "latest" → created_at DESC.
	orderClause := "c.created_at DESC"

	switch f.Sort {
	case "oldest":
		orderClause = "c.created_at ASC"
	case "title":
		orderClause = "c.title ASC"
	}

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
SELECT c.id, c.author_id, c.title, c.slug, c.body, c.body_html,
       c.status, c.visibility, c.view_count, c.comment_count,
       c.published_at, c.created_at, c.updated_at,
       sm.origin_space_id, os.name AS origin_space_name, os.geo_code AS origin_space_code,
       sm.destination_space_id, ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       sm.relocation_year, sm.visa_category, sm.profession,
       sm.biggest_challenge, sm.most_useful_resource, sm.best_advice,
       sm.verified,
       u.username AS author_name, u.username AS author_username, up.display_name AS author_display_name, up.avatar_url AS author_avatar
FROM contents c
JOIN story_meta sm ON sm.content_id = c.id
LEFT JOIN spaces os ON os.id = sm.origin_space_id
JOIN spaces ds ON ds.id = sm.destination_space_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
WHERE %s
ORDER BY %s
LIMIT $%d OFFSET $%d
`, whereClause, orderClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("story: admin list stories: %w", err)
	}
	defer rows.Close()

	var out []StoryRow

	for rows.Next() {
		var r StoryRow
		if err := rows.Scan(
			&r.ID, &r.AuthorID, &r.Title, &r.Slug, &r.Body, &r.BodyHTML,
			&r.Status, &r.Visibility, &r.ViewCount, &r.CommentCount,
			&r.PublishedAt, &r.CreatedAt, &r.UpdatedAt,
			&r.OriginSpaceID, &r.OriginSpaceName, &r.OriginSpaceCode,
			&r.DestSpaceID, &r.DestSpaceName, &r.DestSpaceCode,
			&r.RelocationYear, &r.VisaCategory, &r.Profession,
			&r.BiggestChallenge, &r.MostUsefulResource, &r.BestAdvice,
			&r.Verified,
			&r.AuthorName, &r.AuthorUsername, &r.AuthorDisplayName, &r.AuthorAvatar,
		); err != nil {
			return nil, 0, fmt.Errorf("story: admin scan story row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("story: admin iterate story rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM contents c
JOIN story_meta sm ON sm.content_id = c.id
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("story: admin count stories: %w", err)
	}

	return out, total, nil
}
