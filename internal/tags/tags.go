// Package tags — general cross-domain tags (Sections 08, 49).
// // Distinct from forum_tags (forum-scoped, internal/forum): this package
// backs the tags table content is tagged against via content_tags, and
// followed via the follows package (entity_type="tag").
// // Tag moderation model: all tag mutations (create/update/delete) require
// the tags.manage permission, held by admins and super_admins. The
// tag_moderators table grants no standalone permission node, but since the
// studio moderation spec (.jeos/specs/2026-09-23-studio-moderation-tab.md)
// it resolves dynamically in the moderator queue: tag moderators see and
// can action reports on content carrying their tags (content actions only —
// warn/ban require space/platform/assigned scope). Forum-tag moderators arrive
// through the same table — forum tags are universal since migration 000719.
// Ref: §08, §26.
package tags

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Service provides tag read and admin-write access.
type Service struct {
	read  *gen.Queries
	write *gen.Queries
}

// NewService creates a Service backed by the given database.
func NewService(database *db.DB) *Service {
	return &Service{read: gen.New(database.Reader), write: gen.New(database.Writer)}
}

// ListTags returns every tag for the platform, alphabetically.
func (s *Service) ListTags(ctx context.Context) ([]gen.ListTagsRow, error) {
	tags, err := s.read.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}

	return tags, nil
}

// ListTagsPaginated returns tags filtered by search, sorted, and offset-paginated.
func (s *Service) ListTagsPaginated(ctx context.Context, params gen.ListTagsPaginatedParams) ([]gen.ListTagsPaginatedRow, error) {
	tags, err := s.read.ListTagsPaginated(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("listing tags paginated: %w", err)
	}

	return tags, nil
}

// CountTagsPaginated returns the count of tags matching the search query.
func (s *Service) CountTagsPaginated(ctx context.Context, q string) (int, error) {
	count, err := s.read.CountTagsPaginated(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("counting tags paginated: %w", err)
	}

	return int(count), nil
}

// AdminListTags returns tags filtered by search, sort, and offset-paginated for admin listing.
func (s *Service) AdminListTags(ctx context.Context, params gen.AdminListTagsParams) ([]gen.AdminListTagsRow, error) {
	tags, err := s.read.AdminListTags(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("admin listing tags: %w", err)
	}

	return tags, nil
}

// AdminCountTags returns the count of tags matching a search query and filter.
func (s *Service) AdminCountTags(ctx context.Context, params gen.AdminCountTagsParams) (int, error) {
	count, err := s.read.AdminCountTags(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("admin counting tags: %w", err)
	}

	return int(count), nil
}

// GetTagBySlug returns a single tag by its slug.
func (s *Service) GetTagBySlug(ctx context.Context, slug string) (gen.GetTagBySlugRow, error) {
	tag, err := s.read.GetTagBySlug(ctx, slug)
	if err != nil {
		return gen.GetTagBySlugRow{}, fmt.Errorf("getting tag by slug: %w", err)
	}

	return tag, nil
}

// GetTagByID returns a single tag by its ID (used by DeleteTag guard).
func (s *Service) GetTagByID(ctx context.Context, id pgtype.UUID) (gen.GetTagByIDRow, error) {
	tag, err := s.read.GetTagByID(ctx, id)
	if err != nil {
		return gen.GetTagByIDRow{}, fmt.Errorf("getting tag by id: %w", err)
	}

	return tag, nil
}

// ResolveTagSlugs resolves multiple tag slugs to their IDs in one query (Section 14).
func (s *Service) ResolveTagSlugs(ctx context.Context, slugs []string) ([]gen.GetTagsBySlugsRow, error) {
	rows, err := s.read.GetTagsBySlugs(ctx, slugs)
	if err != nil {
		return nil, fmt.Errorf("resolving tag slugs: %w", err)
	}

	return rows, nil
}

// ResolveTagSlugMap resolves tag slugs to a slug→userID map (content.TagSlugResolver adapter).
func (s *Service) ResolveTagSlugMap(ctx context.Context, slugs []string) (map[string]pgtype.UUID, error) {
	rows, err := s.ResolveTagSlugs(ctx, slugs)
	if err != nil {
		return nil, err
	}

	m := make(map[string]pgtype.UUID, len(rows))
	for _, row := range rows {
		m[row.Slug] = row.ID
	}

	return m, nil
}

// GetOrCreateTagSlugs resolves tag slugs to their IDs, creating any slug that
// does not yet exist as a new platform-scoped tag (name = slug, slug = slug).
// This backs the spec-compliant POST /content/{id}/tags { tag_slugs: [] }
// flow where the composer sends user-typed tag names that may not exist yet
// (Section 14). Returns a slug→tagID map covering every input slug.
// // Concurrent calls for the same slug are safe: if CreateTag fails with a
// unique-violation (the tag was just created by a parallel request), the
// slug is re-resolved instead of returning an error.
func (s *Service) GetOrCreateTagSlugs(ctx context.Context, slugs []string) (map[string]pgtype.UUID, error) {
	rows, err := s.ResolveTagSlugs(ctx, slugs)
	if err != nil {
		return nil, fmt.Errorf("resolving tag slugs: %w", err)
	}

	m := make(map[string]pgtype.UUID, len(slugs))
	for _, row := range rows {
		m[row.Slug] = row.ID
	}

	for _, slug := range slugs {
		if _, exists := m[slug]; exists {
			continue
		}

		tag, cerr := s.CreateTag(ctx, CreateTagInput{
			Name:     slug,
			Slug:     slug,
			IsSystem: false,
		})
		if cerr != nil {
			// A concurrent call may have just created this tag. Re-resolve
			// the single slug; if it now exists, use it. Otherwise propagate.
			retry, rerr := s.ResolveTagSlugs(ctx, []string{slug})
			if rerr == nil && len(retry) > 0 {
				m[slug] = retry[0].ID
				continue
			}

			return nil, fmt.Errorf("creating tag %q: %w", slug, cerr)
		}

		m[slug] = tag.ID
	}

	return m, nil
}

// CountContentByTag returns the number of content items tagged with the given tag.
func (s *Service) CountContentByTag(ctx context.Context, tagID pgtype.UUID) (int, error) {
	count, err := s.read.CountContentByTag(ctx, tagID)
	if err != nil {
		return 0, fmt.Errorf("counting content by tag: %w", err)
	}

	return int(count), nil
}

// CreateTagInput holds parameters for creating a tag (catalog: POST /admin/tags).
type CreateTagInput struct {
	Name         string
	Slug         string
	Description  *string
	Rules        *string
	CoverMediaID pgtype.UUID
	Color        *string
	IsSystem     bool
	LogoMediaID  pgtype.UUID
	FieldValues  []byte
}

// CreateTag creates a new tag.
func (s *Service) CreateTag(ctx context.Context, input CreateTagInput) (gen.CreateTagRow, error) {
	fieldValues := input.FieldValues
	if len(fieldValues) == 0 {
		fieldValues = []byte("{}")
	}

	tag, err := s.write.CreateTag(ctx, gen.CreateTagParams{

		Name:         input.Name,
		Slug:         input.Slug,
		Description:  input.Description,
		Rules:        input.Rules,
		CoverMediaID: input.CoverMediaID,
		Color:        input.Color,
		IsSystem:     input.IsSystem,
		LogoMediaID:  input.LogoMediaID,
		FieldValues:  fieldValues,
	})
	if err != nil {
		return gen.CreateTagRow{}, fmt.Errorf("creating tag: %w", err)
	}

	return tag, nil
}

// UpdateTagInput holds parameters for updating a tag (catalog: PATCH /admin/tags/:id).
type UpdateTagInput struct {
	Name         string
	Slug         string
	Description  *string
	Rules        *string
	CoverMediaID pgtype.UUID
	Color        *string
	LogoMediaID  pgtype.UUID
	IsSystem     bool
	FieldValues  []byte
}

// UpdateTag updates an existing tag's name/slug/description/rules/cover/color/logo/is_system/fields.
func (s *Service) UpdateTag(ctx context.Context, id pgtype.UUID, input UpdateTagInput) (gen.UpdateTagRow, error) {
	fieldValues := input.FieldValues
	if len(fieldValues) == 0 {
		fieldValues = []byte("{}")
	}

	tag, err := s.write.UpdateTag(ctx, gen.UpdateTagParams{
		ID: id,

		Name:         input.Name,
		Slug:         input.Slug,
		Description:  input.Description,
		Rules:        input.Rules,
		CoverMediaID: input.CoverMediaID,
		Color:        input.Color,
		LogoMediaID:  input.LogoMediaID,
		IsSystem:     input.IsSystem,
		FieldValues:  fieldValues,
	})
	if err != nil {
		return gen.UpdateTagRow{}, fmt.Errorf("updating tag: %w", err)
	}

	return tag, nil
}

// DeleteTag deletes a tag (catalog: DELETE /admin/tags/:id).
func (s *Service) DeleteTag(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DeleteTag(ctx, id); err != nil {
		return fmt.Errorf("deleting tag: %w", err)
	}

	return nil
}

// ListTagModerators returns the moderators of a tag (Section 49).
func (s *Service) ListTagModerators(ctx context.Context, tagID pgtype.UUID) ([]gen.ListTagModeratorsRow, error) {
	rows, err := s.read.ListTagModerators(ctx, tagID)
	if err != nil {
		return nil, fmt.Errorf("listing tag moderators: %w", err)
	}

	return rows, nil
}

// AddTagModeratorInput holds parameters for assigning a tag moderator.
type AddTagModeratorInput struct {
	TagID  pgtype.UUID
	UserID pgtype.UUID

	AssignedBy       string
	AssignedByUserID pgtype.UUID
}

// AddTagModerator assigns a user as a moderator of a tag (Section 49).
// Idempotent — re-adding an existing moderator is a no-op refresh.
func (s *Service) AddTagModerator(ctx context.Context, input AddTagModeratorInput) (gen.TagModerator, error) {
	row, err := s.write.AddTagModerator(ctx, gen.AddTagModeratorParams{

		TagID:            input.TagID,
		UserID:           input.UserID,
		AssignedBy:       input.AssignedBy,
		AssignedByUserID: input.AssignedByUserID,
	})
	if err != nil {
		return gen.TagModerator{}, fmt.Errorf("adding tag moderator: %w", err)
	}

	return row, nil
}

// RemoveTagModerator removes a user as a moderator of a tag (Section 49).
func (s *Service) RemoveTagModerator(ctx context.Context, tagID, userID pgtype.UUID) error {
	if err := s.write.RemoveTagModerator(ctx, gen.RemoveTagModeratorParams{

		TagID:  tagID,
		UserID: userID,
	}); err != nil {
		return fmt.Errorf("removing tag moderator: %w", err)
	}

	return nil
}

// GetUserIDByUsername resolves a username to a user ID within the platform.
func (s *Service) GetUserIDByUsername(ctx context.Context, username string) (pgtype.UUID, error) {
	var id pgtype.UUID

	row, err := s.read.GetUserByUsername(ctx,

		username,
	)
	if err != nil {
		return id, fmt.Errorf("looking up user %q: %w", username, err)
	}

	return row.UserID, nil
}
