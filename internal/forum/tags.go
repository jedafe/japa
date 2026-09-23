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

// ErrTagInUse is returned when deleting a tag that still has content attached.
// Ref: Section 08 Admin Tags — "delete tag (only if no content tagged)".
var ErrTagInUse = errors.New("tag is still attached to content")

// Forum tags are universal (Sections 08, 14): they live in the shared `tags`
// and `content_tags` tables, attached to a thread through its backing
// contents row (forum_threads.content_id). The legacy forum_tags /
// forum_thread_tags tables were backfilled by migration 000719.

// CreateTagInput holds parameters for creating a new tag.
type CreateTagInput struct {
	Name            string
	Slug            string
	Description     string
	Rules           string
	CoverImageID    pgtype.UUID
	ModeratorUserID pgtype.UUID
}

// CreateTag creates a new universal tag.
func (s *Service) CreateTag(ctx context.Context, input CreateTagInput) (gen.ForumTag, error) {
	if input.Name == "" {
		return gen.ForumTag{}, errors.New("tag name is required")
	}

	if input.Slug == "" {
		return gen.ForumTag{}, errors.New("tag slug is required")
	}

	tag, err := s.store.createTag(ctx, gen.CreateTagParams{
		Name:         input.Name,
		Slug:         input.Slug,
		Description:  strPtr(input.Description),
		Rules:        strPtr(input.Rules),
		CoverMediaID: input.CoverImageID,
		IsSystem:     false,
		FieldValues:  []byte("{}"),
	})
	if err != nil {
		return gen.ForumTag{}, fmt.Errorf("creating tag: %w", err)
	}

	// Assign the requested moderator through the universal tag_moderators
	// table. BUG-017: assigning a tag moderator is a capability grant —
	// invalidate the new moderator's permission cache so the grant is
	// visible promptly. Ref: Rule 6 + Section 03.
	if input.ModeratorUserID.Valid {
		if err := s.store.addTagModerator(ctx, tag.ID, input.ModeratorUserID, input.ModeratorUserID); err != nil {
			slog.Warn("failed to assign tag moderator", "tag_id", tag.ID, "error", err)
		} else {
			tag.ModeratorUserID = input.ModeratorUserID
			s.invalidateModeratorCache(ctx, input.ModeratorUserID)
		}
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
// tags.content_count is maintained by the trg_tags_content_count_sync
// trigger on content_tags. Ref: Section 08 Tags (max 5 tags per post).
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
	}

	return nil
}

// RemoveThreadTag removes a tag from a thread.
func (s *Service) RemoveThreadTag(ctx context.Context, threadID, tagID pgtype.UUID) error {
	if err := s.store.removeThreadTag(ctx, threadID, tagID); err != nil {
		return fmt.Errorf("removing tag from thread: %w", err)
	}

	return nil
}

// UpdateTagInput holds parameters for updating a tag.
type UpdateTagInput struct {
	Name            string
	Slug            string
	Description     string
	Rules           string
	CoverImageID    pgtype.UUID
	ModeratorUserID pgtype.UUID
}

// UpdateTag updates a universal tag's mutable fields, preserving the fields
// the forum surface does not manage (color, is_system, field_values,
// logo_media_id).
func (s *Service) UpdateTag(ctx context.Context, id pgtype.UUID, input UpdateTagInput) (gen.ForumTag, error) {
	// Load existing tag to fill in unchanged fields.
	existing, err := s.store.getUniversalTagByID(ctx, id)
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

	description := existing.Description
	if input.Description != "" {
		description = strPtr(input.Description)
	}

	rules := existing.Rules
	if input.Rules != "" {
		rules = strPtr(input.Rules)
	}

	coverImageID := existing.CoverMediaID
	if input.CoverImageID.Valid {
		coverImageID = input.CoverImageID
	}

	// Resolve the current moderator (first row) so a change can be detected.
	var existingModerator pgtype.UUID
	if mods, merr := s.store.listTagModerators(ctx, id); merr == nil && len(mods) > 0 {
		existingModerator = mods[0].UserID
	}

	moderatorUserID := existingModerator
	if input.ModeratorUserID.Valid {
		moderatorUserID = input.ModeratorUserID
	}

	updated, err := s.store.updateTag(ctx, gen.UpdateTagParams{
		ID:           id,
		Name:         name,
		Slug:         slug,
		Description:  description,
		Rules:        rules,
		CoverMediaID: coverImageID,
		Color:        existing.Color,
		LogoMediaID:  existing.LogoMediaID,
		IsSystem:     existing.IsSystem,
		FieldValues:  existing.FieldValues,
	})
	if err != nil {
		return gen.ForumTag{}, err
	}

	// BUG-017: invalidate the permission cache for the previous and the new
	// moderator whenever the assignment changes. Ref: Rule 6.
	if !uuidEqual(existingModerator, moderatorUserID) {
		if existingModerator.Valid {
			if err := s.store.removeTagModerator(ctx, id, existingModerator); err != nil {
				slog.Warn("failed to remove tag moderator", "tag_id", id, "error", err)
			} else {
				s.invalidateModeratorCache(ctx, existingModerator)
			}
		}

		if moderatorUserID.Valid {
			if err := s.store.addTagModerator(ctx, id, moderatorUserID, moderatorUserID); err != nil {
				slog.Warn("failed to assign tag moderator", "tag_id", id, "error", err)
			} else {
				updated.ModeratorUserID = moderatorUserID
				s.invalidateModeratorCache(ctx, moderatorUserID)
			}
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

// DeleteTag deletes a universal tag. Per Section 08 (Admin Tags), a tag may
// only be deleted while no content references it.
func (s *Service) DeleteTag(ctx context.Context, id pgtype.UUID) error {
	count, err := s.store.countContentByTag(ctx, id)
	if err != nil {
		return err
	}

	if count > 0 {
		return ErrTagInUse
	}

	return s.store.deleteTag(ctx, id)
}

// strPtr returns a *string. Returns nil for empty strings.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
