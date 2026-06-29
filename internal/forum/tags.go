package forum

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// CreateTagInput holds parameters for creating a new forum tag.
type CreateTagInput struct {
	Name             string
	Slug             string
	Description      string
	Rules            string
	CoverImageID     pgtype.UUID
	ModeratorUserID  pgtype.UUID
}

// CreateTag creates a new forum tag.
func (s *Service) CreateTag(ctx context.Context, tenantID pgtype.UUID, input CreateTagInput) (gen.ForumTag, error) {
	if input.Name == "" {
		return gen.ForumTag{}, fmt.Errorf("tag name is required")
	}
	if input.Slug == "" {
		return gen.ForumTag{}, fmt.Errorf("tag slug is required")
	}

	var coverImageID, moderatorUserID pgtype.UUID
	if input.CoverImageID.Valid {
		coverImageID = input.CoverImageID
	}
	if input.ModeratorUserID.Valid {
		moderatorUserID = input.ModeratorUserID
	}

	tag, err := s.store.createTag(ctx, gen.CreateForumTagParams{
		TenantID:         tenantID,
		Name:             input.Name,
		Slug:             input.Slug,
		Description:      strPtr(input.Description),
		Rules:            strPtr(input.Rules),
		CoverImageID:     coverImageID,
		ModeratorUserID:  moderatorUserID,
	})
	if err != nil {
		return gen.ForumTag{}, fmt.Errorf("creating forum tag: %w", err)
	}
	return tag, nil
}

// GetTagByID retrieves a tag by ID.
func (s *Service) GetTagByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.ForumTag, error) {
	return s.store.getTagByID(ctx, id, tenantID)
}

// GetTagBySlug retrieves a tag by slug.
func (s *Service) GetTagBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.ForumTag, error) {
	return s.store.getTagBySlug(ctx, slug, tenantID)
}

// ListTags returns paginated tags.
func (s *Service) ListTags(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ForumTag, error) {
	return s.store.listTags(ctx, tenantID, limit, offset)
}

// AddThreadTag adds a tag to a thread.
func (s *Service) AddThreadTag(ctx context.Context, threadID, tagID pgtype.UUID) error {
	if err := s.store.addThreadTag(ctx, threadID, tagID); err != nil {
		return fmt.Errorf("adding tag to thread: %w", err)
	}
	// Increment the tag's follower count.
	if err := s.store.updateTagFollowerCount(ctx, tagID, pgtype.UUID{}, 1); err != nil {
		slog.Warn("failed to increment tag follower count", "error", err)
	}
	return nil
}

// RemoveThreadTag removes a tag from a thread.
func (s *Service) RemoveThreadTag(ctx context.Context, threadID, tagID pgtype.UUID) error {
	if err := s.store.removeThreadTag(ctx, threadID, tagID); err != nil {
		return fmt.Errorf("removing tag from thread: %w", err)
	}
	// Decrement the tag's follower count.
	if err := s.store.updateTagFollowerCount(ctx, tagID, pgtype.UUID{}, -1); err != nil {
		slog.Warn("failed to decrement tag follower count", "error", err)
	}
	return nil
}

// strPtr returns a *string. Returns nil for empty strings.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
