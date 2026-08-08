package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/seo"
	"github.com/pgvector/pgvector-go"
)

// ---------------------------------------------------------------------------
// Contents — CRUD
// ---------------------------------------------------------------------------

// CreateContentInput holds the parameters for creating a new content row.
// The Service layer derives slug uniqueness, read time, and content hash
// from these inputs.
type CreateContentInput struct {
	AuthorID        pgtype.UUID
	OrganizationID  pgtype.UUID
	SpaceID         pgtype.UUID
	ContentType     string
	Title           string
	Slug            string
	Body            string
	BodyHtml        string //nolint:revive // matches sqlc-generated field name
	FieldValues     []byte
	ContentHash     *string
	GeoSpaceID      pgtype.UUID
	HeaderImageID   pgtype.UUID
	FeaturedImageID pgtype.UUID
	CanonicalUrl    *string //nolint:revive // matches sqlc-generated field name
	MetaTitle       *string
	MetaDescription *string
	OgImageUrl      *string //nolint:revive // matches sqlc-generated field name
	SeoIndexable    bool
	Status          string
	Visibility      string
	PublishAt       pgtype.Timestamptz
	GuideCategory   *string
}

// CreateContent creates a new content row, ensuring slug uniqueness within
// the tenant. If the provided slug is empty, one is derived from the title.
// The content hash and read time are computed automatically from the body.
func (s *Service) CreateContent(ctx context.Context, tenantID pgtype.UUID, input CreateContentInput) (gen.CreateContentRow, error) {
	slug := input.Slug
	if slug == "" {
		slug = Slugify(input.Title)
	}

	uniqueSlug, err := s.EnsureUniqueSlug(ctx, slug, tenantID)
	if err != nil {
		return gen.CreateContentRow{}, fmt.Errorf("ensuring unique slug: %w", err)
	}

	fieldValues := input.FieldValues
	if fieldValues == nil {
		fieldValues = []byte("{}")
	}

	// Compute content hash from body if not provided.
	// Skip hashing for empty bodies — the unique index has
	// `WHERE content_hash IS NULL`, so NULL hashes don't collide.
	// Without this, every empty-body draft gets the same hash and
	// the autosave's first fire (before the user types body content)
	// hits a 409 duplicate_content (Rule 12 / §41).
	contentHash := input.ContentHash
	if contentHash == nil && strings.TrimSpace(input.Body) != "" {
		h := ComputeContentHash(input.Body)
		contentHash = &h
	}

	// Post-as-org (ADR-062): attributing to an org requires the author be an
	// owner/admin/editor of it. author_id remains the human, always.
	if input.OrganizationID.Valid {
		role, err := s.read.GetOrganizationMemberRole(ctx, gen.GetOrganizationMemberRoleParams{
			OrganizationID: input.OrganizationID,
			UserID:         input.AuthorID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return gen.CreateContentRow{}, ErrOrgPostForbidden
			}
			return gen.CreateContentRow{}, fmt.Errorf("checking org membership: %w", err)
		}
		if !isOrgPostRole(role) {
			return gen.CreateContentRow{}, ErrOrgPostForbidden
		}
	}

	content, err := s.write.CreateContent(ctx, gen.CreateContentParams{
		TenantID:        tenantID,
		AuthorID:        input.AuthorID,
		OrganizationID:  input.OrganizationID,
		SpaceID:         input.SpaceID,
		ContentType:     input.ContentType,
		Title:           input.Title,
		Slug:            uniqueSlug,
		Body:            input.Body,
		BodyHtml:        input.BodyHtml,
		FieldValues:     fieldValues,
		ContentHash:     contentHash,
		GeoSpaceID:      input.GeoSpaceID,
		HeaderImageID:   input.HeaderImageID,
		FeaturedImageID: input.FeaturedImageID,
		CanonicalUrl:    input.CanonicalUrl,
		MetaTitle:       input.MetaTitle,
		MetaDescription: input.MetaDescription,
		OgImageUrl:      input.OgImageUrl,
		SeoIndexable:    input.SeoIndexable,
		Status:          input.Status,
		Visibility:      input.Visibility,
		ReadTimeMinutes: ComputeReadTime(input.Body),
		PublishAt:       input.PublishAt,
		GuideCategory:   input.GuideCategory,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return gen.CreateContentRow{}, ErrDuplicateContent
		}
		return gen.CreateContentRow{}, fmt.Errorf("creating content: %w", err)
	}

	// Create the initial revision.
	if _, err := s.write.CreateContentRevision(ctx, gen.CreateContentRevisionParams{
		ContentID:   content.ID,
		FieldValues: fieldValues,
		Title:       input.Title,
		Body:        input.Body,
		BodyHtml:    input.BodyHtml,
		EditedBy:    input.AuthorID,
	}); err != nil {
		return gen.CreateContentRow{}, fmt.Errorf("creating initial revision: %w", err)
	}

	return content, nil
}

// GetContent returns the content with the given ID, scoped to the tenant.
func (s *Service) GetContent(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetContentByIDRow, error) {
	content, err := s.read.GetContentByID(ctx, gen.GetContentByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.GetContentByIDRow{}, fmt.Errorf("getting content: %w", err)
	}

	return content, nil
}

// GetContentBySlug returns the content matching the given slug, scoped to
// the tenant.
func (s *Service) GetContentBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.GetContentBySlugRow, error) {
	content, err := s.read.GetContentBySlug(ctx, gen.GetContentBySlugParams{
		Slug:     slug,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.GetContentBySlugRow{}, fmt.Errorf("getting content by slug: %w", err)
	}

	return content, nil
}

// ListContents returns a page of content for the given tenant, newest first.
// Pass a zero-value CursorCreatedAt for the first page. When q is non-empty,
// results are filtered by case-insensitive match on title, slug, or body.
func (s *Service) ListContents(ctx context.Context, tenantID pgtype.UUID, q string, cursor pgtype.Timestamptz, limit int32) ([]gen.ListContentsByTenantRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	contents, err := s.read.ListContentsByTenant(ctx, gen.ListContentsByTenantParams{
		TenantID:        tenantID,
		Q:               q,
		CursorCreatedAt: cursor,
		PageLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing contents: %w", err)
	}

	return contents, nil
}

// ListContentsByType returns a page of content filtered by content_type.
func (s *Service) ListContentsByType(ctx context.Context, tenantID pgtype.UUID, contentType string, cursor pgtype.Timestamptz, limit int32) ([]gen.ListContentsByTypeRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	contents, err := s.read.ListContentsByType(ctx, gen.ListContentsByTypeParams{
		TenantID:        tenantID,
		ContentType:     contentType,
		CursorCreatedAt: cursor,
		PageLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing contents by type: %w", err)
	}

	return contents, nil
}

// ListContentsBySpace returns a page of content filtered by space_id.
func (s *Service) ListContentsBySpace(ctx context.Context, tenantID, spaceID pgtype.UUID, cursor pgtype.Timestamptz, limit int32) ([]gen.ListContentsBySpaceRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	contents, err := s.read.ListContentsBySpace(ctx, gen.ListContentsBySpaceParams{
		TenantID:        tenantID,
		SpaceID:         spaceID,
		CursorCreatedAt: cursor,
		PageLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing contents by space: %w", err)
	}

	return contents, nil
}

// SearchContentsBySpace returns content within a space whose title matches the
// query (case-insensitive substring). Used by the space search tab
// (GET /spaces/{slug}/content?q=...). Offset-paginated.
func (s *Service) SearchContentsBySpace(ctx context.Context, tenantID, spaceID pgtype.UUID, query string, limit, offset int32) ([]gen.SearchContentsBySpaceRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	if offset < 0 {
		offset = 0
	}

	contents, err := s.read.SearchContentsBySpace(ctx, gen.SearchContentsBySpaceParams{
		TenantID:   tenantID,
		SpaceID:    spaceID,
		Query:      &query,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("searching contents by space: %w", err)
	}

	return contents, nil
}

// UpdateContentInput holds the parameters for updating an existing content row.
type UpdateContentInput struct {
	Title           string
	Slug            string
	Body            string
	BodyHtml        string //nolint:revive // matches sqlc-generated field name
	FieldValues     []byte
	ContentHash     *string
	GeoSpaceID      pgtype.UUID
	HeaderImageID   pgtype.UUID
	FeaturedImageID pgtype.UUID
	CanonicalUrl    *string //nolint:revive // matches sqlc-generated field name
	MetaTitle       *string
	MetaDescription *string
	OgImageUrl      *string //nolint:revive // matches sqlc-generated field name
	SeoIndexable    *bool
	Status          string
	Visibility      string
	PublishAt       pgtype.Timestamptz
	IsLocked        bool
	IsPinned        bool
	OrganizationID  pgtype.UUID
	GuideCategory   *string
}

// UpdateContent updates an existing content row, recording a revision and
// tracking slug changes in slug_history. The old slug is preserved for
// 301 redirects.
func (s *Service) UpdateContent(ctx context.Context, id, tenantID, editedBy pgtype.UUID, input UpdateContentInput, bypassEditWindow bool) (gen.UpdateContentRow, error) {
	existing, err := s.read.GetContentByID(ctx, gen.GetContentByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.UpdateContentRow{}, fmt.Errorf("getting existing content for update: %w", err)
	}

	// Edit-window enforcement (BL-008 / Section 49): the original author may
	// only edit within `content.edit_window_minutes` of creation. Moderators
	// and admins editing someone else's post bypass this check because they
	// are not the author. Super admins bypass even on their own content.
	// A zero/missing created_at is treated as "no limit".
	if !bypassEditWindow && existing.AuthorID.Valid && existing.CreatedAt.Valid &&
		existing.AuthorID.Bytes == editedBy.Bytes {
		windowMinutes := defaultEditWindowMinutes
		if s.settings != nil {
			windowMinutes = s.settings.GetIntOr(ctx, tenantID, "content.edit_window_minutes", defaultEditWindowMinutes)
		}
		if windowMinutes > 0 {
			deadline := existing.CreatedAt.Time.Add(time.Duration(windowMinutes) * time.Minute)
			if time.Now().After(deadline) {
				return gen.UpdateContentRow{}, ErrEditWindowExpired
			}
		}
	}

	newSlug := input.Slug
	if newSlug == "" {
		newSlug = existing.Slug
	} else if newSlug != existing.Slug {
		uniqueSlug, err := s.EnsureUniqueSlug(ctx, newSlug, tenantID)
		if err != nil {
			return gen.UpdateContentRow{}, fmt.Errorf("ensuring unique slug on update: %w", err)
		}

		newSlug = uniqueSlug
	}

	fieldValues := input.FieldValues
	if fieldValues == nil {
		fieldValues = existing.FieldValues
	}

	// Post-as-org (ADR-062): attributing/keeping an org on this post requires the
	// editor be an owner/admin/editor of that org. Symmetric with CreateContent.
	if input.OrganizationID.Valid {
		role, err := s.read.GetOrganizationMemberRole(ctx, gen.GetOrganizationMemberRoleParams{
			OrganizationID: input.OrganizationID,
			UserID:         editedBy,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return gen.UpdateContentRow{}, ErrOrgPostForbidden
			}
			return gen.UpdateContentRow{}, fmt.Errorf("checking org membership on update: %w", err)
		}
		if !isOrgPostRole(role) {
			return gen.UpdateContentRow{}, ErrOrgPostForbidden
		}
	}

	// Duplicate-content pre-check (Rule 12 / BL-006): if a content_hash is
	// supplied, verify no other row in this tenant already has it. The unique
	// index idx_contents_tenant_content_hash is the ultimate guard, but this
	// yields a clean 409 duplicate_content instead of a raw 23505 violation.
	if input.ContentHash != nil && *input.ContentHash != "" {
		if _, err := s.read.GetContentByHash(ctx, gen.GetContentByHashParams{
			TenantID:    tenantID,
			ContentHash: input.ContentHash,
			ExcludeID:   id,
		}); err == nil {
			return gen.UpdateContentRow{}, ErrDuplicateContent
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return gen.UpdateContentRow{}, fmt.Errorf("checking content hash uniqueness: %w", err)
		}
	}

	content, err := s.write.UpdateContent(ctx, gen.UpdateContentParams{
		OrganizationID:  input.OrganizationID,
		Title:           input.Title,
		Slug:            newSlug,
		Body:            input.Body,
		BodyHtml:        input.BodyHtml,
		FieldValues:     fieldValues,
		ContentHash:     input.ContentHash,
		GeoSpaceID:      input.GeoSpaceID,
		HeaderImageID:   input.HeaderImageID,
		FeaturedImageID: input.FeaturedImageID,
		CanonicalUrl:    input.CanonicalUrl,
		MetaTitle:       input.MetaTitle,
		MetaDescription: input.MetaDescription,
		OgImageUrl:      input.OgImageUrl,
		SeoIndexable:    input.SeoIndexable,
		Status:          input.Status,
		Visibility:      input.Visibility,
		ReadTimeMinutes: ComputeReadTime(input.Body),
		PublishAt:       input.PublishAt,
		IsLocked:        input.IsLocked,
		IsPinned:        input.IsPinned,
		GuideCategory:   input.GuideCategory,
		ID:              id,
		TenantID:        tenantID,
	})
	if err != nil {
		return gen.UpdateContentRow{}, fmt.Errorf("updating content: %w", err)
	}

	// Record slug change for 301 redirect.
	s.RecordSlugChange(ctx, EntityContent, id, existing.Slug, newSlug)

	// Create a revision.
	if _, err := s.write.CreateContentRevision(ctx, gen.CreateContentRevisionParams{
		ContentID:   id,
		FieldValues: fieldValues,
		Title:       input.Title,
		Body:        input.Body,
		BodyHtml:    input.BodyHtml,
		EditedBy:    editedBy,
	}); err != nil {
		return gen.UpdateContentRow{}, fmt.Errorf("creating revision: %w", err)
	}

	return content, nil
}

// UpdateContentStatus changes the status of a content row (e.g. draft to
// published). When transitioning to 'published', published_at is stamped
// if it was previously null.
func (s *Service) UpdateContentStatus(ctx context.Context, id, tenantID pgtype.UUID, status string) (gen.UpdateContentStatusRow, error) {
	content, err := s.write.UpdateContentStatus(ctx, gen.UpdateContentStatusParams{
		Status:   status,
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.UpdateContentStatusRow{}, fmt.Errorf("updating content status: %w", err)
	}

	// Ensure a content_scores row exists when content is published. The feed
	// queries (explore/top/rising/space/profile) INNER JOIN content_scores, so
	// a missing row excludes the content from those feeds entirely. The row is
	// created with zero defaults; the engagement worker increments it as
	// views/votes/comments/bookmarks arrive. Ref: Section 32, Guide §21.
	if status == StatusPublished {
		if err := s.write.EnsureContentScore(ctx, content.ID); err != nil {
			// Non-fatal: a missing score row degrades ranking but does not
			// break the publish. Log and continue.
			slog.Warn("content: failed to ensure content_scores row on publish",
				"content_id", content.ID.String(), "error", err)
		}
	}

	return content, nil
}

// ScheduleContent sets status='scheduled' and publish_at for a future publish
// time, without touching title/body/other fields (catalog: POST
// /content/:id/schedule). Callers must also enqueue the scheduled_publish
// Asynq task with ProcessAt=publishAt for the actual publish to happen.
func (s *Service) ScheduleContent(ctx context.Context, id, tenantID pgtype.UUID, publishAt time.Time) (gen.ScheduleContentRow, error) {
	content, err := s.write.ScheduleContent(ctx, gen.ScheduleContentParams{
		PublishAt: pgtype.Timestamptz{Time: publishAt, Valid: true},
		ID:        id,
		TenantID:  tenantID,
	})
	if err != nil {
		return gen.ScheduleContentRow{}, fmt.Errorf("scheduling content: %w", err)
	}

	return content, nil
}

// IncrementContentView increments the view count of a content row.
func (s *Service) IncrementContentView(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.IncrementContentView(ctx, gen.IncrementContentViewParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("incrementing content view: %w", err)
	}

	return nil
}

// RecordContentViewWithDetails increments the view count AND records a
// deduplicated row in user_content_views with country_code and session_id
// (Section 14 — view tracking). The user_content_views PK deduplicates
// per user/session per day; ON CONFLICT DO NOTHING prevents duplicates.
// For anonymous users, user_id is invalid and session_id is used for
// deduplication.
func (s *Service) RecordContentViewWithDetails(ctx context.Context, id, tenantID, userID pgtype.UUID, sessionID, countryCode *string) error {
	if err := s.write.IncrementContentView(ctx, gen.IncrementContentViewParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("incrementing content view: %w", err)
	}

	// Best-effort: a failure to record the detailed view row should not
	// block the view count increment (the count is already updated above).
	if err := s.write.RecordContentView(ctx, gen.RecordContentViewParams{
		ContentID:   id,
		UserID:      userID,
		SessionID:   sessionID,
		CountryCode: countryCode,
	}); err != nil {
		slog.Warn("recording detailed content view",
			"error", err, "content_id", id.String())
	}

	return nil
}

// IncrementContentCommentCount increments the denormalized comment_count on a
// content row. Called when a comment is created so feed cards (which read
// contents.comment_count) show accurate counts immediately. Ref: Section 32.
func (s *Service) IncrementContentCommentCount(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.IncrementContentCommentCount(ctx, gen.IncrementContentCommentCountParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("incrementing content comment count: %w", err)
	}

	return nil
}

// DecrementContentCommentCount decrements the denormalized comment_count on a
// content row (clamped at 0). Called when a comment is removed.
func (s *Service) DecrementContentCommentCount(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DecrementContentCommentCount(ctx, gen.DecrementContentCommentCountParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("decrementing content comment count: %w", err)
	}

	return nil
}

// EnsureContentScore creates a zero-default content_scores row if one does not
// yet exist. Idempotent (ON CONFLICT DO NOTHING). Required so INNER JOIN feed
// queries do not exclude content and so the engagement worker can increment
// counts in place. Ref: Section 32, Guide §21.
func (s *Service) EnsureContentScore(ctx context.Context, contentID pgtype.UUID) error {
	if err := s.write.EnsureContentScore(ctx, contentID); err != nil {
		return fmt.Errorf("ensuring content score: %w", err)
	}

	return nil
}

// DeleteContent soft-deletes a content row, scoped to the tenant.
func (s *Service) DeleteContent(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteContent(ctx, gen.DeleteContentParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting content: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Content Revisions
// ---------------------------------------------------------------------------

// ListContentRevisions returns the full revision history for a content row,
// newest first.
func (s *Service) ListContentRevisions(ctx context.Context, contentID pgtype.UUID) ([]gen.ContentRevision, error) {
	revisions, err := s.read.ListContentRevisions(ctx, contentID)
	if err != nil {
		return nil, fmt.Errorf("listing content revisions: %w", err)
	}

	return revisions, nil
}

// GetContentRevision returns a specific revision by ID.
func (s *Service) GetContentRevision(ctx context.Context, id, contentID pgtype.UUID) (gen.ContentRevision, error) {
	revision, err := s.read.GetContentRevision(ctx, gen.GetContentRevisionParams{
		ID:        id,
		ContentID: contentID,
	})
	if err != nil {
		return gen.ContentRevision{}, fmt.Errorf("getting content revision: %w", err)
	}

	return revision, nil
}

// ---------------------------------------------------------------------------
// Space Pinned Posts
// ---------------------------------------------------------------------------

// PinContent pins a content row to a space. The pinnedBy user and position
// are recorded. If the content is already pinned, the unique constraint
// prevents a duplicate.
func (s *Service) PinContent(ctx context.Context, tenantID, spaceID, contentID, pinnedBy pgtype.UUID, position int32) (gen.PinContentToSpaceRow, error) {
	maxPinned := 5
	if s.settings != nil {
		maxPinned = s.settings.GetIntOr(ctx, tenantID, "spaces.max_pinned_posts", 5)
	}

	if maxPinned > 0 {
		existing, err := s.read.ListPinnedContent(ctx, spaceID)
		if err != nil {
			return gen.PinContentToSpaceRow{}, fmt.Errorf("checking pinned count: %w", err)
		}
		if len(existing) >= maxPinned {
			return gen.PinContentToSpaceRow{}, fmt.Errorf("max_pinned_posts_reached: space already has %d pinned posts (limit %d)", len(existing), maxPinned)
		}
	}

	pin, err := s.write.PinContentToSpace(ctx, gen.PinContentToSpaceParams{
		SpaceID:   spaceID,
		ContentID: contentID,
		PinnedBy:  pinnedBy,
		Position:  position,
	})
	if err != nil {
		return gen.PinContentToSpaceRow{}, fmt.Errorf("pinning content to space: %w", err)
	}

	return pin, nil
}

// UnpinContent removes a pin from a space.
func (s *Service) UnpinContent(ctx context.Context, spaceID, contentID pgtype.UUID) error {
	if err := s.write.UnpinContentFromSpace(ctx, gen.UnpinContentFromSpaceParams{
		SpaceID:   spaceID,
		ContentID: contentID,
	}); err != nil {
		return fmt.Errorf("unpinning content from space: %w", err)
	}

	return nil
}

// ListPinnedContent returns all pinned posts for a space, ordered by position.
func (s *Service) ListPinnedContent(ctx context.Context, spaceID pgtype.UUID) ([]gen.ListPinnedContentRow, error) {
	pins, err := s.read.ListPinnedContent(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("listing pinned content: %w", err)
	}

	return pins, nil
}

// IsContentPinned returns true if the given content is pinned to the space.
func (s *Service) IsContentPinned(ctx context.Context, spaceID, contentID pgtype.UUID) (bool, error) {
	pinned, err := s.read.IsContentPinned(ctx, gen.IsContentPinnedParams{
		SpaceID:   spaceID,
		ContentID: contentID,
	})
	if err != nil {
		return false, fmt.Errorf("checking if content is pinned: %w", err)
	}

	return pinned, nil
}

// ---------------------------------------------------------------------------
// Reactions
// ---------------------------------------------------------------------------

// ListReactionTypes returns all reaction types for a tenant, ordered by position.
func (s *Service) ListReactionTypes(ctx context.Context, tenantID pgtype.UUID) ([]gen.ReactionType, error) {
	types, err := s.read.ListReactionTypes(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing reaction types: %w", err)
	}

	return types, nil
}

// GetReactionType returns a single reaction type by tenant and key.
func (s *Service) GetReactionType(ctx context.Context, tenantID pgtype.UUID, key string) (gen.ReactionType, error) {
	rt, err := s.read.GetReactionType(ctx, gen.GetReactionTypeParams{
		TenantID: tenantID,
		Key:      key,
	})
	if err != nil {
		return gen.ReactionType{}, fmt.Errorf("getting reaction type: %w", err)
	}

	return rt, nil
}

// CreateReactionType creates a new reaction type with the given key, emoji,
// label, and position for a specific tenant.
func (s *Service) CreateReactionType(ctx context.Context, tenantID pgtype.UUID, key, emoji, label string, enabled bool, position int32) (gen.ReactionType, error) {
	rt, err := s.write.CreateReactionType(ctx, gen.CreateReactionTypeParams{
		TenantID: tenantID,
		Key:      key,
		Emoji:    emoji,
		Label:    label,
		Enabled:  enabled,
		Position: position,
	})
	if err != nil {
		return gen.ReactionType{}, fmt.Errorf("creating reaction type: %w", err)
	}

	return rt, nil
}

// UpdateReactionType updates an existing reaction type by tenant and key.
func (s *Service) UpdateReactionType(ctx context.Context, tenantID pgtype.UUID, key, emoji, label string, enabled bool, position int32) (gen.ReactionType, error) {
	rt, err := s.write.UpdateReactionType(ctx, gen.UpdateReactionTypeParams{
		Emoji:    emoji,
		Label:    label,
		Enabled:  enabled,
		Position: position,
		TenantID: tenantID,
		Key:      key,
	})
	if err != nil {
		return gen.ReactionType{}, fmt.Errorf("updating reaction type: %w", err)
	}

	return rt, nil
}

// DeleteReactionType removes a reaction type by tenant and key. Existing reactions
// referencing the key are cascade-deleted by the FK constraint.
func (s *Service) DeleteReactionType(ctx context.Context, tenantID pgtype.UUID, key string) error {
	if err := s.write.DeleteReactionType(ctx, gen.DeleteReactionTypeParams{
		TenantID: tenantID,
		Key:      key,
	}); err != nil {
		return fmt.Errorf("deleting reaction type: %w", err)
	}

	return nil
}

// AddReaction adds a reaction from a user to an entity. Idempotent — if the
// user already reacted with the same key, nothing changes (ON CONFLICT DO NOTHING).
func (s *Service) AddReaction(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID, userID pgtype.UUID, reactionKey string) (gen.Reaction, error) {
	reaction, err := s.write.AddReaction(ctx, gen.AddReactionParams{
		TenantID:    tenantID,
		EntityType:  entityType,
		EntityID:    entityID,
		UserID:      userID,
		ReactionKey: reactionKey,
	})
	if err != nil {
		return gen.Reaction{}, fmt.Errorf("adding reaction: %w", err)
	}

	return reaction, nil
}

// RemoveReaction removes a user's reaction from an entity.
func (s *Service) RemoveReaction(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID, userID pgtype.UUID, reactionKey string) error {
	if err := s.write.RemoveReaction(ctx, gen.RemoveReactionParams{
		TenantID:    tenantID,
		EntityType:  entityType,
		EntityID:    entityID,
		UserID:      userID,
		ReactionKey: reactionKey,
	}); err != nil {
		return fmt.Errorf("removing reaction: %w", err)
	}

	return nil
}

// ListReactionsByEntity returns all reactions for a given entity.
func (s *Service) ListReactionsByEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) ([]gen.Reaction, error) {
	reactions, err := s.read.ListReactionsByEntity(ctx, gen.ListReactionsByEntityParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing reactions by entity: %w", err)
	}

	return reactions, nil
}

// CountReactionsByEntity returns reaction counts grouped by reaction key.
func (s *Service) CountReactionsByEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) ([]gen.CountReactionsByEntityRow, error) {
	counts, err := s.read.CountReactionsByEntity(ctx, gen.CountReactionsByEntityParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return nil, fmt.Errorf("counting reactions by entity: %w", err)
	}

	return counts, nil
}

// GetUserReactions returns all reactions a specific user has on an entity.
func (s *Service) GetUserReactions(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID, userID pgtype.UUID) ([]gen.Reaction, error) {
	reactions, err := s.read.GetUserReactions(ctx, gen.GetUserReactionsParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
		UserID:     userID,
	})
	if err != nil {
		return nil, fmt.Errorf("getting user reactions: %w", err)
	}

	return reactions, nil
}

// ---------------------------------------------------------------------------
// JSON-LD Templates
// ---------------------------------------------------------------------------

// GetJSONLDTemplate returns the active JSON-LD template for a content type
// within a tenant.
func (s *Service) GetJSONLDTemplate(ctx context.Context, tenantID pgtype.UUID, contentType string) (gen.ContentTypeJsonldTemplate, error) {
	tmpl, err := s.read.GetJSONLDTemplate(ctx, gen.GetJSONLDTemplateParams{
		TenantID:    tenantID,
		ContentType: contentType,
	})
	if err != nil {
		return gen.ContentTypeJsonldTemplate{}, fmt.Errorf("getting JSON-LD template: %w", err)
	}

	return tmpl, nil
}

// ResolveJSONLD fetches the active admin-managed JSON-LD template for the
// given content type, interpolates the supplied field values into the
// {{field_key}} placeholders, and returns the ready-to-emit JSON-LD. When no
// active template exists for the content type, it returns (nil, nil) so the
// caller can fall back to a hardcoded generator (Section 14, Section 26).
func (s *Service) ResolveJSONLD(ctx context.Context, tenantID pgtype.UUID, contentType string, fields map[string]string) (json.RawMessage, error) {
	tmpl, err := s.GetJSONLDTemplate(ctx, tenantID, contentType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	body, err := seo.InterpolateJSONLD(tmpl.Template, fields)
	if err != nil {
		return nil, fmt.Errorf("interpolating JSON-LD template for %q: %w", contentType, err)
	}
	return body, nil
}

// ListJSONLDTemplates returns all JSON-LD templates for a tenant, optionally
// filtered by q (matches content_type or template body).
func (s *Service) ListJSONLDTemplates(ctx context.Context, tenantID pgtype.UUID, q pgtype.Text) ([]gen.ContentTypeJsonldTemplate, error) {
	templates, err := s.read.ListJSONLDTemplates(ctx, gen.ListJSONLDTemplatesParams{
		TenantID: tenantID,
		Q:        q.String,
	})
	if err != nil {
		return nil, fmt.Errorf("listing JSON-LD templates: %w", err)
	}

	return templates, nil
}

// UpsertJSONLDTemplate creates or updates a JSON-LD template for a content
// type within a tenant.
func (s *Service) UpsertJSONLDTemplate(ctx context.Context, tenantID pgtype.UUID, contentType string, template []byte, isActive bool) (gen.ContentTypeJsonldTemplate, error) {
	tmpl, err := s.write.UpsertJSONLDTemplate(ctx, gen.UpsertJSONLDTemplateParams{
		TenantID:    tenantID,
		ContentType: contentType,
		Template:    template,
		IsActive:    isActive,
	})
	if err != nil {
		return gen.ContentTypeJsonldTemplate{}, fmt.Errorf("upserting JSON-LD template: %w", err)
	}

	return tmpl, nil
}

// DeleteJSONLDTemplate removes a JSON-LD template by ID, scoped to tenant.
func (s *Service) DeleteJSONLDTemplate(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteJSONLDTemplate(ctx, gen.DeleteJSONLDTemplateParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting JSON-LD template: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Content Scores
// ---------------------------------------------------------------------------

// UpsertContentScore creates or updates the score row for a content item.
func (s *Service) UpsertContentScore(ctx context.Context, contentID pgtype.UUID, voteScore, commentCount, viewCount, bookmarkCount int32, velocityScore pgtype.Numeric) (gen.ContentScore, error) {
	score, err := s.write.UpsertContentScore(ctx, gen.UpsertContentScoreParams{
		ContentID:     contentID,
		VoteScore:     voteScore,
		CommentCount:  commentCount,
		ViewCount:     viewCount,
		BookmarkCount: bookmarkCount,
		VelocityScore: velocityScore,
	})
	if err != nil {
		return gen.ContentScore{}, fmt.Errorf("upserting content score: %w", err)
	}

	return score, nil
}

// GetContentScore returns the score row for a content item.
func (s *Service) GetContentScore(ctx context.Context, contentID pgtype.UUID) (gen.ContentScore, error) {
	score, err := s.read.GetContentScore(ctx, contentID)
	if err != nil {
		return gen.ContentScore{}, fmt.Errorf("getting content score: %w", err)
	}

	return score, nil
}

// ---------------------------------------------------------------------------
// Content Tags
// ---------------------------------------------------------------------------

// AddContentTag associates a tag with content. Idempotent (ON CONFLICT DO NOTHING).
func (s *Service) AddContentTag(ctx context.Context, contentID, tagID pgtype.UUID) error {
	if err := s.write.AddContentTag(ctx, gen.AddContentTagParams{
		ContentID: contentID,
		TagID:     tagID,
	}); err != nil {
		return fmt.Errorf("adding content tag: %w", err)
	}

	return nil
}

// RemoveContentTag disassociates a tag from content.
func (s *Service) RemoveContentTag(ctx context.Context, contentID, tagID pgtype.UUID) error {
	if err := s.write.RemoveContentTag(ctx, gen.RemoveContentTagParams{
		ContentID: contentID,
		TagID:     tagID,
	}); err != nil {
		return fmt.Errorf("removing content tag: %w", err)
	}

	return nil
}

// ListContentTags returns all tags associated with the given content.
func (s *Service) ListContentTags(ctx context.Context, contentID pgtype.UUID) ([]gen.ListContentTagsRow, error) {
	tags, err := s.read.ListContentTags(ctx, contentID)
	if err != nil {
		return nil, fmt.Errorf("listing content tags: %w", err)
	}

	return tags, nil
}

// ListContentByTag returns published content items tagged with the given tag.
func (s *Service) ListContentByTag(ctx context.Context, tenantID, tagID pgtype.UUID, cursor pgtype.Timestamptz, limit int32) ([]gen.ListContentByTagRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	contents, err := s.read.ListContentByTag(ctx, gen.ListContentByTagParams{
		TagID:           tagID,
		TenantID:        tenantID,
		CursorCreatedAt: cursor,
		PageLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing content by tag: %w", err)
	}

	return contents, nil
}

// ---------------------------------------------------------------------------
// Content Shares
// ---------------------------------------------------------------------------

// ShareContent records a share event and returns the created share.
func (s *Service) ShareContent(ctx context.Context, tenantID, contentID, userID pgtype.UUID, platform string) (gen.ContentShare, error) {
	if platform == "" {
		platform = "link"
	}

	share, err := s.write.CreateContentShare(ctx, gen.CreateContentShareParams{
		TenantID:  tenantID,
		ContentID: contentID,
		UserID:    userID,
		Platform:  platform,
	})
	if err != nil {
		return gen.ContentShare{}, fmt.Errorf("sharing content: %w", err)
	}

	return share, nil
}

// CountContentShares returns the total share count for content.
func (s *Service) CountContentShares(ctx context.Context, tenantID, contentID pgtype.UUID) (int32, error) {
	count, err := s.read.CountContentShares(ctx, gen.CountContentSharesParams{
		TenantID:  tenantID,
		ContentID: contentID,
	})
	if err != nil {
		return 0, fmt.Errorf("counting content shares: %w", err)
	}

	return count, nil
}

// ---------------------------------------------------------------------------
// Bookmarks
// ---------------------------------------------------------------------------

// CreateBookmark saves a bookmark for the given entity. Idempotent — upserts
// folder. folderID is the optional FK to bookmark_folders (BL-011); pass a
// zero pgtype.UUID for "unfiled".
func (s *Service) CreateBookmark(ctx context.Context, tenantID, userID pgtype.UUID, entityType string, entityID pgtype.UUID, folder string, folderID pgtype.UUID) (gen.CreateBookmarkRow, error) {
	if folder == "" {
		folder = "default"
	}

	bm, err := s.write.CreateBookmark(ctx, gen.CreateBookmarkParams{
		TenantID:   tenantID,
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Folder:     folder,
		FolderID:   folderID,
	})
	if err != nil {
		return gen.CreateBookmarkRow{}, fmt.Errorf("creating bookmark: %w", err)
	}

	return bm, nil
}

// SetBookmarkFolder moves an existing bookmark into a folder (BL-011).
// Pass a zero pgtype.UUID to unfile the bookmark.
func (s *Service) SetBookmarkFolder(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, folderID pgtype.UUID) error {
	if err := s.write.SetBookmarkFolder(ctx, gen.SetBookmarkFolderParams{
		FolderID:   folderID,
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
	}); err != nil {
		return fmt.Errorf("setting bookmark folder: %w", err)
	}
	return nil
}

// ValidateFolderOwnership checks that a bookmark folder belongs to the given
// user and tenant. Returns nil if the folder exists and is owned by the user.
// Returns ErrNotFound if the folder does not exist or is owned by another user.
// A zero/invalid folderID is treated as "unfiled" and returns nil (BL-013).
func (s *Service) ValidateFolderOwnership(ctx context.Context, folderID, tenantID, userID pgtype.UUID) error {
	if !folderID.Valid || folderID.Bytes == [16]byte{} {
		return nil
	}

	_, err := s.read.GetBookmarkFolder(ctx, gen.GetBookmarkFolderParams{
		ID:       folderID,
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return fmt.Errorf("%w: folder not owned by user", errpkg.ErrNotFound)
	}
	return nil
}

// ListBookmarksByFolder returns bookmarks filed under a specific folder (BL-011).
func (s *Service) ListBookmarksByFolder(ctx context.Context, tenantID, userID, folderID pgtype.UUID, limit, offset int32) ([]gen.ListBookmarksByFolderRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	bms, err := s.read.ListBookmarksByFolder(ctx, gen.ListBookmarksByFolderParams{
		UserID:   userID,
		TenantID: tenantID,
		FolderID: folderID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing bookmarks by folder: %w", err)
	}
	return bms, nil
}

// RemoveBookmark deletes a user's bookmark for the given entity.
func (s *Service) RemoveBookmark(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	if err := s.write.DeleteBookmark(ctx, gen.DeleteBookmarkParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
	}); err != nil {
		return fmt.Errorf("deleting bookmark: %w", err)
	}

	return nil
}

// IsBookmarked checks if a user has bookmarked the given entity.
func (s *Service) IsBookmarked(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
	result, err := s.read.IsBookmarked(ctx, gen.IsBookmarkedParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return false, fmt.Errorf("checking bookmark: %w", err)
	}

	return result, nil
}

// ListBookmarks returns all bookmarks for a user.
func (s *Service) ListBookmarks(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.ListBookmarksByUserRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	bms, err := s.read.ListBookmarksByUser(ctx, gen.ListBookmarksByUserParams{
		TenantID:   tenantID,
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing bookmarks: %w", err)
	}

	return bms, nil
}

// ---------------------------------------------------------------------------
// Related Content (pgvector cosine similarity)
// ---------------------------------------------------------------------------

// GetRelatedContent returns up to limit content items most similar to the given
// content, ranked by cosine distance on the relevance_embedding vector.
func (s *Service) GetRelatedContent(ctx context.Context, tenantID, excludeID pgtype.UUID, embedding pgvector.Vector, limit int32) ([]gen.GetRelatedContentRow, error) {
	if limit <= 0 || limit > 12 {
		limit = 12
	}

	contents, err := s.read.GetRelatedContent(ctx, gen.GetRelatedContentParams{
		TenantID:  tenantID,
		ExcludeID: excludeID,
		Embedding: &embedding,
		PageLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("getting related content: %w", err)
	}

	return contents, nil
}

// ---------------------------------------------------------------------------
// Content Spaces (multi-space assignment — Section 14)
// ---------------------------------------------------------------------------

// UpdateContentSpaces replaces the full set of space assignments for a
// content item. The primary space is set on the contents.geo_space_id
// column for backward-compat query compatibility, and a content_spaces
// row is inserted for each assigned space (exactly one with is_primary
// = true). The entire operation runs in a transaction.
func (s *Service) UpdateContentSpaces(ctx context.Context, contentID, primarySpaceID pgtype.UUID, additionalSpaceIDs []pgtype.UUID) error {
	return db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		// Clear existing assignments.
		if err := q.DeleteContentSpaces(ctx, contentID); err != nil {
			return fmt.Errorf("deleting content_spaces: %w", err)
		}

		// Insert primary space.
		if err := q.AddContentSpace(ctx, gen.AddContentSpaceParams{
			ContentID: contentID,
			SpaceID:   primarySpaceID,
			IsPrimary: true,
		}); err != nil {
			return fmt.Errorf("adding primary content_space: %w", err)
		}

		// Insert additional spaces.
		for _, sid := range additionalSpaceIDs {
			if isSameUUID(sid, primarySpaceID) {
				continue // don't duplicate the primary
			}
			if err := q.AddContentSpace(ctx, gen.AddContentSpaceParams{
				ContentID: contentID,
				SpaceID:   sid,
				IsPrimary: false,
			}); err != nil {
				return fmt.Errorf("adding content_space: %w", err)
			}
		}

		return nil
	})
}

// isSameUUID returns true when two pgtype.UUID values are equal and valid.
func isSameUUID(a, b pgtype.UUID) bool {
	return a.Valid && b.Valid && a.Bytes == b.Bytes
}

// SetContentSpaceID updates only the space_id column on a content item.
// Used by the auto-assign feature to set the primary space after keyword
// matching. Ref: sections/05-geo-first-space-system.md
func (s *Service) SetContentSpaceID(ctx context.Context, contentID, spaceID pgtype.UUID) error {
	return s.write.SetContentSpaceID(ctx, gen.SetContentSpaceIDParams{
		ContentID: contentID,
		SpaceID:   spaceID,
	})
}
