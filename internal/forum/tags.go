package forum

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ErrTooManyTags is returned when adding tags would exceed the per-thread tag cap.
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (BUG-008).
var ErrTooManyTags = errors.New("too many tags: maximum tags per thread exceeded")

// CreateTagInput holds parameters for creating a new forum tag.
type CreateTagInput struct {
	Name            string
	Slug            string
	Description     string
	Rules           string
	CoverImageID    pgtype.UUID
	ModeratorUserID pgtype.UUID
}

// CreateTag creates a new forum tag.
func (s *Service) CreateTag(ctx context.Context, input CreateTagInput) (gen.ForumTag, error) {
	if input.Name == "" {
		return gen.ForumTag{}, errors.New("tag name is required")
	}

	if input.Slug == "" {
		return gen.ForumTag{}, errors.New("tag slug is required")
	}

	var coverImageID, moderatorUserID pgtype.UUID
	if input.CoverImageID.Valid {
		coverImageID = input.CoverImageID
	}

	if input.ModeratorUserID.Valid {
		moderatorUserID = input.ModeratorUserID
	}

	tag, err := s.store.createTag(ctx, gen.CreateForumTagParams{

		Name:            input.Name,
		Slug:            input.Slug,
		Description:     strPtr(input.Description),
		Rules:           strPtr(input.Rules),
		CoverImageID:    coverImageID,
		ModeratorUserID: moderatorUserID,
	})
	if err != nil {
		return gen.ForumTag{}, fmt.Errorf("creating forum tag: %w", err)
	}

	// BUG-017: assigning a tag moderator is a capability grant — invalidate
	// the new moderator's permission cache so the grant is visible promptly.
	// Ref: Rule 6 + Section 03 (Permission Cache Invalidation).
	if moderatorUserID.Valid {
		s.invalidateModeratorCache(ctx, moderatorUserID)
	}

	return tag, nil
}

// GetTagByID retrieves a tag by ID.
func (s *Service) GetTagByID(ctx context.Context, id pgtype.UUID) (gen.ForumTag, error) {
	return s.store.getTagByID(ctx, id)
}

// GetTagBySlug retrieves a tag by slug.
func (s *Service) GetTagBySlug(ctx context.Context, slug string) (gen.ForumTag, error) {
	return s.store.getTagBySlug(ctx, slug)
}

// ListTags returns paginated tags.
func (s *Service) ListTags(ctx context.Context, limit, offset int32) ([]gen.ForumTag, error) {
	return s.store.listTags(ctx, limit, offset)
}

// maxTagsForThread resolves the per-thread tag cap from system_settings
// (content.max_tags), falling back to MaxTagsPerThread when settings are
// not wired. Ref: AGENTS.md Rule 3, Section 08 Tags.
func (s *Service) maxTagsForThread(ctx context.Context) int32 {
	if s.settings != nil {
		// #nosec G115 // value bounded by caller
		return int32(s.settings.GetIntOr(ctx, "content.max_tags", int(MaxTagsPerThread)))
	}

	return MaxTagsPerThread
}

// AddThreadTag adds a tag to a thread. Enforces the per-thread tag cap.
// Ref: Section 08 Tags (max 5 tags per post).
func (s *Service) AddThreadTag(ctx context.Context, threadID, tagID pgtype.UUID) error {
	// Enforce the per-thread tag cap before inserting.
	maxTags := s.maxTagsForThread(ctx)

	count, err := s.store.countThreadTags(ctx, threadID)
	if err != nil {
		return fmt.Errorf("counting thread tags: %w", err)
	}

	if count >= maxTags {
		return fmt.Errorf("thread already has the maximum of %d tags", maxTags)
	}

	if err := s.store.addThreadTag(ctx, threadID, tagID); err != nil {
		return fmt.Errorf("adding tag to thread: %w", err)
	}
	// Increment the tag's follower count.
	if err := s.store.updateTagFollowerCount(ctx, tagID, 1); err != nil {
		slog.Warn("failed to increment tag follower count", "error", err)
	}

	return nil
}

// AddThreadTagsBulk adds multiple tags to a thread in one call
// (POST /forum/{id}/tags). Enforces the per-thread tag cap across the
// combined set. Ref: api-routes-catalog L596.
func (s *Service) AddThreadTagsBulk(ctx context.Context, threadID pgtype.UUID, tagIDs []pgtype.UUID) error {
	maxTags := s.maxTagsForThread(ctx)

	existing, err := s.store.countThreadTags(ctx, threadID)
	if err != nil {
		return fmt.Errorf("counting thread tags: %w", err)
	}
	// De-duplicate the incoming ids against each other.
	seen := make(map[[16]byte]struct{}, len(tagIDs))
	unique := make([]pgtype.UUID, 0, len(tagIDs))

	for _, id := range tagIDs {
		if !id.Valid {
			continue
		}

		if _, ok := seen[id.Bytes]; ok {
			continue
		}

		seen[id.Bytes] = struct{}{}

		unique = append(unique, id)
	}

	//nolint:gosec // G115: bounded
	if existing+int32(len(unique)) > maxTags { // #nosec G115 -- len() of a deduped tag slice, bounded by maxTags //nolint:gosec // G115: bounded
		return fmt.Errorf("adding these tags would exceed the maximum of %d tags per thread", maxTags)
	}

	for _, tagID := range unique {
		if err := s.store.addThreadTag(ctx, threadID, tagID); err != nil {
			return fmt.Errorf("adding tag to thread: %w", err)
		}

		if err := s.store.updateTagFollowerCount(ctx, tagID, 1); err != nil {
			slog.Warn("failed to increment tag follower count", "error", err)
		}
	}

	return nil
}

// RemoveThreadTag removes a tag from a thread.
func (s *Service) RemoveThreadTag(ctx context.Context, threadID, tagID pgtype.UUID) error {
	if err := s.store.removeThreadTag(ctx, threadID, tagID); err != nil {
		return fmt.Errorf("removing tag from thread: %w", err)
	}
	// Decrement the tag's follower count.
	if err := s.store.updateTagFollowerCount(ctx, tagID, -1); err != nil {
		slog.Warn("failed to decrement tag follower count", "error", err)
	}

	return nil
}

// UpdateTagInput holds parameters for updating a forum tag.
type UpdateTagInput struct {
	Name            string
	Slug            string
	Description     string
	Rules           string
	CoverImageID    pgtype.UUID
	ModeratorUserID pgtype.UUID
}

// UpdateTag updates a forum tag's mutable fields.
func (s *Service) UpdateTag(ctx context.Context, id pgtype.UUID, input UpdateTagInput) (gen.ForumTag, error) {
	// Load existing tag to fill in unchanged fields.
	existing, err := s.store.getTagByID(ctx, id)
	if err != nil {
		return gen.ForumTag{}, fmt.Errorf("getting tag for update: %w", err)
	}

	name := existing.Name
	if input.Name != "" {
		name = input.Name
	}

	slug := existing.Slug
	if input.Slug != "" {
		slug = input.Slug
	}

	description := ""
	if existing.Description != nil {
		description = *existing.Description
	}

	if input.Description != "" {
		description = input.Description
	}

	rules := ""
	if existing.Rules != nil {
		rules = *existing.Rules
	}

	if input.Rules != "" {
		rules = input.Rules
	}

	coverImageID := existing.CoverImageID
	if input.CoverImageID.Valid {
		coverImageID = input.CoverImageID
	}

	moderatorUserID := existing.ModeratorUserID
	if input.ModeratorUserID.Valid {
		moderatorUserID = input.ModeratorUserID
	}

	updated, err := s.store.updateTag(ctx, id, name, slug, description, rules, coverImageID, moderatorUserID)
	if err != nil {
		return gen.ForumTag{}, err
	}

	// BUG-017: invalidate the permission cache for the previous and the new
	// moderator whenever the assignment changes. Ref: Rule 6.
	if !uuidEqual(existing.ModeratorUserID, moderatorUserID) {
		if existing.ModeratorUserID.Valid {
			s.invalidateModeratorCache(ctx, existing.ModeratorUserID)
		}

		if moderatorUserID.Valid {
			s.invalidateModeratorCache(ctx, moderatorUserID)
		}
	}

	return updated, nil
}

// invalidateModeratorCache clears the user's permission cache. No-op when
// the perm-cache invalidator was not injected. Ref: Rule 6.
func (s *Service) invalidateModeratorCache(ctx context.Context, userID pgtype.UUID) {
	if s.permCache == nil {
		return
	}

	if err := s.permCache.InvalidatePermCache(ctx, userID); err != nil {
		slog.Warn("failed to invalidate tag moderator perm cache", "user_id", userID, "error", err)
	}
}

// uuidEqual reports whether two pgtype.UUID values are equal.
func uuidEqual(a, b pgtype.UUID) bool {
	return a.Valid == b.Valid && a.Bytes == b.Bytes
}

// DeleteTag soft-deletes a forum tag.
func (s *Service) DeleteTag(ctx context.Context, id pgtype.UUID) error {
	return s.store.softDeleteTag(ctx, id)
}

// strPtr returns a *string. Returns nil for empty strings.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
