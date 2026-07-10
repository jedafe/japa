package content

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/pgvector/pgvector-go"
)

// ---------------------------------------------------------------------------
// Contents — CRUD
// ---------------------------------------------------------------------------

// CreateContentInput holds the parameters for creating a new content row.
// The Service layer derives slug uniqueness, read time, and content hash
// from these inputs.
type CreateContentInput struct {
	AuthorID     pgtype.UUID
	SpaceID      pgtype.UUID
	ContentType  string
	Title        string
	Slug         string
	Body         string
	BodyHtml     string //nolint:revive // matches sqlc-generated field name
	FieldValues  []byte
	ContentHash  *string
	GeoSpaceID   pgtype.UUID
	CanonicalUrl *string //nolint:revive // matches sqlc-generated field name
	Status       string
	Visibility   string
	PublishAt    pgtype.Timestamptz
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
	contentHash := input.ContentHash
	if contentHash == nil {
		h := ComputeContentHash(input.Body)
		contentHash = &h
	}

	content, err := s.write.CreateContent(ctx, gen.CreateContentParams{
		TenantID:        tenantID,
		AuthorID:        input.AuthorID,
		SpaceID:         input.SpaceID,
		ContentType:     input.ContentType,
		Title:           input.Title,
		Slug:            uniqueSlug,
		Body:            input.Body,
		BodyHtml:        input.BodyHtml,
		FieldValues:     fieldValues,
		ContentHash:     contentHash,
		GeoSpaceID:      input.GeoSpaceID,
		CanonicalUrl:    input.CanonicalUrl,
		Status:          input.Status,
		Visibility:      input.Visibility,
		ReadTimeMinutes: ComputeReadTime(input.Body),
		PublishAt:       input.PublishAt,
	})
	if err != nil {
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
// Pass a zero-value CursorCreatedAt for the first page.
func (s *Service) ListContents(ctx context.Context, tenantID pgtype.UUID, cursor pgtype.Timestamptz, limit int32) ([]gen.ListContentsByTenantRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	contents, err := s.read.ListContentsByTenant(ctx, gen.ListContentsByTenantParams{
		TenantID:        tenantID,
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

// UpdateContentInput holds the parameters for updating an existing content row.
type UpdateContentInput struct {
	Title        string
	Slug         string
	Body         string
	BodyHtml     string //nolint:revive // matches sqlc-generated field name
	FieldValues  []byte
	ContentHash  *string
	GeoSpaceID   pgtype.UUID
	CanonicalUrl *string //nolint:revive // matches sqlc-generated field name
	Status       string
	Visibility   string
	PublishAt    pgtype.Timestamptz
	IsLocked     bool
	IsPinned     bool
}

// UpdateContent updates an existing content row, recording a revision and
// tracking slug changes in slug_history. The old slug is preserved for
// 301 redirects.
func (s *Service) UpdateContent(ctx context.Context, id, tenantID, editedBy pgtype.UUID, input UpdateContentInput) (gen.UpdateContentRow, error) {
	existing, err := s.read.GetContentByID(ctx, gen.GetContentByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.UpdateContentRow{}, fmt.Errorf("getting existing content for update: %w", err)
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

	content, err := s.write.UpdateContent(ctx, gen.UpdateContentParams{
		Title:           input.Title,
		Slug:            newSlug,
		Body:            input.Body,
		BodyHtml:        input.BodyHtml,
		FieldValues:     fieldValues,
		ContentHash:     input.ContentHash,
		GeoSpaceID:      input.GeoSpaceID,
		CanonicalUrl:    input.CanonicalUrl,
		Status:          input.Status,
		Visibility:      input.Visibility,
		ReadTimeMinutes: ComputeReadTime(input.Body),
		PublishAt:       input.PublishAt,
		IsLocked:        input.IsLocked,
		IsPinned:        input.IsPinned,
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

// DeleteContent removes a content row, scoped to the tenant.
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
func (s *Service) PinContent(ctx context.Context, spaceID, contentID, pinnedBy pgtype.UUID, position int32) (gen.PinContentToSpaceRow, error) {
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

// ListJSONLDTemplates returns all JSON-LD templates for a tenant.
func (s *Service) ListJSONLDTemplates(ctx context.Context, tenantID pgtype.UUID) ([]gen.ContentTypeJsonldTemplate, error) {
	templates, err := s.read.ListJSONLDTemplates(ctx, tenantID)
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

// CreateBookmark saves a bookmark for the given entity. Idempotent — upserts folder.
func (s *Service) CreateBookmark(ctx context.Context, tenantID, userID pgtype.UUID, entityType string, entityID pgtype.UUID, folder string) (gen.Bookmark, error) {
	if folder == "" {
		folder = "default"
	}

	bm, err := s.write.CreateBookmark(ctx, gen.CreateBookmarkParams{
		TenantID:   tenantID,
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Folder:     folder,
	})
	if err != nil {
		return gen.Bookmark{}, fmt.Errorf("creating bookmark: %w", err)
	}

	return bm, nil
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
func (s *Service) ListBookmarks(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.Bookmark, error) {
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
