// Package story — service.go orchestrates story submission, editing,
// deletion, filtered listing, and admin verification. It delegates content
// row creation to the content.Service (§14) and manages the story_meta
// extension table via the local Store. Feature-flagged via story.enabled
// system_setting (§79).
//
// Ref: japaconnect-engineering/sections/79-community-story-wall.md
//      japaconnect-engineering/adr/ADR-087-community-story-wall.md
package story

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
)

// ContentService is the subset of content.Service the story package needs.
// Defined as an interface so the story package depends on a narrow contract,
// not the full content service.
type ContentService interface {
	CreateContent(ctx context.Context, tenantID pgtype.UUID, input content.CreateContentInput) (gen.CreateContentRow, error)
	GetContent(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetContentByIDRow, error)
	UpdateContent(ctx context.Context, id, tenantID, editedBy pgtype.UUID, input content.UpdateContentInput, bypassEditWindow bool) (gen.UpdateContentRow, error)
	DeleteContent(ctx context.Context, id, tenantID pgtype.UUID) error
}

// StoryDTO is the API response shape for a single story.
type StoryDTO struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Slug             string `json:"slug"`
	Body             string `json:"body"`
	BodyHTML         string `json:"body_html"`
	Status           string `json:"status"`
	Visibility       string `json:"visibility"`
	ViewCount        int32  `json:"view_count"`
	CommentCount     int32  `json:"comment_count"`
	PublishedAt      string `json:"published_at"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
	OriginSpaceID    string `json:"origin_space_id,omitempty"`
	OriginSpaceName  string `json:"origin_space_name,omitempty"`
	OriginSpaceCode  string `json:"origin_space_code,omitempty"`
	DestSpaceID      string `json:"destination_space_id"`
	DestSpaceName    string `json:"destination_space_name"`
	DestSpaceCode    string `json:"destination_space_code,omitempty"`
	RelocationYear   int16  `json:"relocation_year,omitempty"`
	VisaCategory     string `json:"visa_category,omitempty"`
	Profession       string `json:"profession,omitempty"`
	BiggestChallenge string `json:"biggest_challenge,omitempty"`
	MostUsefulRes    string `json:"most_useful_resource,omitempty"`
	BestAdvice       string `json:"best_advice,omitempty"`
	Verified         bool   `json:"verified"`
	AuthorName       string `json:"author_name,omitempty"`
	AuthorAvatar     string `json:"author_avatar,omitempty"`
}

// SubmitStoryInput is the request body for POST /stories.
type SubmitStoryInput struct {
	Title              string `json:"title"`
	Body               string `json:"body"`
	OriginSpaceID      string `json:"origin_space_id"`
	DestinationSpaceID string `json:"destination_space_id"`
	RelocationYear     int16  `json:"relocation_year"`
	VisaCategory       string `json:"visa_category"`
	Profession         string `json:"profession"`
	BiggestChallenge   string `json:"biggest_challenge"`
	MostUsefulResource string `json:"most_useful_resource"`
	BestAdvice         string `json:"best_advice"`
}

// EditStoryInput is the request body for PATCH /stories/:id.
type EditStoryInput struct {
	Title              *string `json:"title"`
	Body               *string `json:"body"`
	OriginSpaceID      *string `json:"origin_space_id"`
	DestinationSpaceID *string `json:"destination_space_id"`
	RelocationYear     *int16  `json:"relocation_year"`
	VisaCategory       *string `json:"visa_category"`
	Profession         *string `json:"profession"`
	BiggestChallenge   *string `json:"biggest_challenge"`
	MostUsefulResource *string `json:"most_useful_resource"`
	BestAdvice         *string `json:"best_advice"`
}

// Service holds the business logic for the Community Story Wall.
type Service struct {
	store    *Store
	content  ContentService
	settings *config.Settings
}

// NewService creates a story service.
func NewService(store *Store, contentSvc ContentService, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		content:  contentSvc,
		settings: settings,
	}
}

// Enabled reads the story.enabled system_setting toggle (§79 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "story.enabled", false)
}

// Submit creates a new story — a contents row with content_type='story' plus
// a story_meta row. The story is created in 'draft' status; the author can
// submit for review (set status to 'pending') via the content update flow.
// Auth required (story.create permission checked at route level).
func (s *Service) Submit(ctx context.Context, tenantID, authorID pgtype.UUID, input SubmitStoryInput) (StoryDTO, error) {
	if input.Title == "" || input.DestinationSpaceID == "" {
		return StoryDTO{}, fmt.Errorf("title and destination_space_id are required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return StoryDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var originSpaceID pgtype.UUID
	if input.OriginSpaceID != "" {
		if err := originSpaceID.Scan(input.OriginSpaceID); err != nil {
			return StoryDTO{}, fmt.Errorf("invalid origin_space_id: %w", err)
		}
	}

	contentRow, err := s.content.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:    authorID,
		ContentType: content.TypeStory,
		Title:       input.Title,
		Body:        input.Body,
		Status:      content.StatusDraft,
		Visibility:  content.VisibilityPublic,
		GeoSpaceID:  destSpaceID, // file story in destination country space
	})
	if err != nil {
		return StoryDTO{}, fmt.Errorf("story: create content: %w", err)
	}

	var relocationYear pgtype.Int2
	if input.RelocationYear > 0 {
		relocationYear = pgtype.Int2{Int16: input.RelocationYear, Valid: true}
	}

	meta := StoryMeta{
		TenantID:           tenantID,
		ContentID:          contentRow.ID,
		OriginSpaceID:      originSpaceID,
		DestinationSpaceID: destSpaceID,
		RelocationYear:     relocationYear,
		Verified:           false,
	}
	if input.VisaCategory != "" {
		meta.VisaCategory = &input.VisaCategory
	}
	if input.Profession != "" {
		meta.Profession = &input.Profession
	}
	if input.BiggestChallenge != "" {
		meta.BiggestChallenge = &input.BiggestChallenge
	}
	if input.MostUsefulResource != "" {
		meta.MostUsefulResource = &input.MostUsefulResource
	}
	if input.BestAdvice != "" {
		meta.BestAdvice = &input.BestAdvice
	}

	if err := s.store.CreateStoryMeta(ctx, meta); err != nil {
		slog.Error("story: create story_meta failed after content creation", "content_id", contentRow.ID, "error", err)
		return StoryDTO{}, fmt.Errorf("story: create story_meta: %w", err)
	}

	return s.getStoryDTOByID(ctx, contentRow.ID, tenantID)
}

// GetBySlug returns a single published story by its slug. Anonymous-accessible.
func (s *Service) GetBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (StoryDTO, error) {
	row, err := s.store.GetStoryBySlug(ctx, slug, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return StoryDTO{}, pgx.ErrNoRows
		}
		return StoryDTO{}, fmt.Errorf("story: get by slug: %w", err)
	}
	return rowToDTO(row), nil
}

// List returns a paginated, filtered list of published stories.
func (s *Service) List(ctx context.Context, tenantID pgtype.UUID, originSpaceID, destSpaceID pgtype.UUID, visaCategory, profession, sort string, page, limit int32) ([]StoryDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.ListStories(ctx, ListStoriesFilter{
		TenantID:      tenantID,
		OriginSpaceID: originSpaceID,
		DestSpaceID:   destSpaceID,
		VisaCategory:  visaCategory,
		Profession:    profession,
		Sort:          sort,
		Status:        content.StatusPublished,
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("story: list: %w", err)
	}

	dtos := make([]StoryDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}
	return dtos, total, nil
}

// EditOwn updates the author's own story. The caller must verify ownership
// (story.edit_own permission + author_id match). Only the provided (non-nil)
// fields are updated.
func (s *Service) EditOwn(ctx context.Context, contentID, tenantID, userID pgtype.UUID, input EditStoryInput) (StoryDTO, error) {
	// Fetch existing content to verify ownership.
	existing, err := s.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return StoryDTO{}, pgx.ErrNoRows
		}
		return StoryDTO{}, fmt.Errorf("story: edit: get content: %w", err)
	}
	if existing.AuthorID != userID {
		return StoryDTO{}, ErrNotOwner
	}

	// Build content update input from non-nil fields. UpdateContent
	// overwrites with empty strings, so preserve existing values when
	// the input field is nil.
	updateInput := content.UpdateContentInput{
		Title: existing.Title,
		Body:  existing.Body,
	}
	if input.Title != nil {
		updateInput.Title = *input.Title
	}
	if input.Body != nil {
		updateInput.Body = *input.Body
	}

	if _, err := s.content.UpdateContent(ctx, contentID, tenantID, userID, updateInput, false); err != nil {
		return StoryDTO{}, fmt.Errorf("story: edit: update content: %w", err)
	}

	// Update story_meta fields.
	meta := StoryMeta{}
	if input.OriginSpaceID != nil && *input.OriginSpaceID != "" {
		if err := meta.OriginSpaceID.Scan(*input.OriginSpaceID); err != nil {
			return StoryDTO{}, fmt.Errorf("story: edit: invalid origin_space_id: %w", err)
		}
	}
	if input.DestinationSpaceID != nil && *input.DestinationSpaceID != "" {
		if err := meta.DestinationSpaceID.Scan(*input.DestinationSpaceID); err != nil {
			return StoryDTO{}, fmt.Errorf("story: edit: invalid destination_space_id: %w", err)
		}
	}
	if input.RelocationYear != nil && *input.RelocationYear > 0 {
		meta.RelocationYear = pgtype.Int2{Int16: *input.RelocationYear, Valid: true}
	}
	if input.VisaCategory != nil {
		meta.VisaCategory = input.VisaCategory
	}
	if input.Profession != nil {
		meta.Profession = input.Profession
	}
	if input.BiggestChallenge != nil {
		meta.BiggestChallenge = input.BiggestChallenge
	}
	if input.MostUsefulResource != nil {
		meta.MostUsefulResource = input.MostUsefulResource
	}
	if input.BestAdvice != nil {
		meta.BestAdvice = input.BestAdvice
	}

	if err := s.store.UpdateStoryMeta(ctx, contentID, tenantID, meta); err != nil {
		return StoryDTO{}, fmt.Errorf("story: edit: update story_meta: %w", err)
	}

	return s.getStoryDTOByID(ctx, contentID, tenantID)
}

// DeleteOwn soft-deletes the author's own story. The caller must verify
// ownership (story.delete_own permission + author_id match).
func (s *Service) DeleteOwn(ctx context.Context, contentID, tenantID, userID pgtype.UUID) error {
	existing, err := s.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("story: delete: get content: %w", err)
	}
	if existing.AuthorID != userID {
		return ErrNotOwner
	}

	if err := s.content.DeleteContent(ctx, contentID, tenantID); err != nil {
		return fmt.Errorf("story: delete: %w", err)
	}
	return nil
}

// Verify marks a story as verified by an admin (story.verify permission).
func (s *Service) Verify(ctx context.Context, contentID, tenantID pgtype.UUID) error {
	if err := s.store.VerifyStory(ctx, contentID, tenantID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("story: verify: %w", err)
	}
	return nil
}

// AdminList returns all stories (any status) for the moderation queue.
func (s *Service) AdminList(ctx context.Context, tenantID pgtype.UUID, status, q string, verified *bool, page, limit int32) ([]StoryDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.AdminListStories(ctx, AdminListStoriesFilter{
		TenantID: tenantID,
		Status:   status,
		Verified: verified,
		Q:        q,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("story: admin list: %w", err)
	}

	dtos := make([]StoryDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}
	return dtos, total, nil
}

// getStoryDTOByID fetches a story by content_id and converts to DTO.
func (s *Service) getStoryDTOByID(ctx context.Context, contentID, tenantID pgtype.UUID) (StoryDTO, error) {
	contentRow, err := s.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		return StoryDTO{}, fmt.Errorf("story: get by id: %w", err)
	}
	meta, err := s.store.GetStoryMeta(ctx, contentID, tenantID)
	if err != nil {
		return StoryDTO{}, fmt.Errorf("story: get meta by id: %w", err)
	}
	return metaToDTO(contentRow, meta), nil
}

// ErrNotOwner is returned when a user tries to edit or delete a story they
// did not author.
var ErrNotOwner = fmt.Errorf("not the story author")

// rowToDTO converts a StoryRow (joined query result) to a StoryDTO.
func rowToDTO(r StoryRow) StoryDTO {
	dto := StoryDTO{
		ID:           r.ID.String(),
		Title:        r.Title,
		Slug:         r.Slug,
		Body:         r.Body,
		BodyHTML:     r.BodyHTML,
		Status:       r.Status,
		Visibility:   r.Visibility,
		ViewCount:    r.ViewCount,
		CommentCount: r.CommentCount,
		PublishedAt:  formatTs(r.PublishedAt),
		CreatedAt:    formatTs(r.CreatedAt),
		UpdatedAt:    formatTs(r.UpdatedAt),
		Verified:     r.Verified,
	}
	if r.OriginSpaceID.Valid {
		dto.OriginSpaceID = r.OriginSpaceID.String()
	}
	if r.OriginSpaceName != nil {
		dto.OriginSpaceName = *r.OriginSpaceName
	}
	if r.OriginSpaceCode != nil {
		dto.OriginSpaceCode = *r.OriginSpaceCode
	}
	if r.DestSpaceID.Valid {
		dto.DestSpaceID = r.DestSpaceID.String()
	}
	if r.DestSpaceName != nil {
		dto.DestSpaceName = *r.DestSpaceName
	}
	if r.DestSpaceCode != nil {
		dto.DestSpaceCode = *r.DestSpaceCode
	}
	if r.RelocationYear.Valid {
		dto.RelocationYear = r.RelocationYear.Int16
	}
	if r.VisaCategory != nil {
		dto.VisaCategory = *r.VisaCategory
	}
	if r.Profession != nil {
		dto.Profession = *r.Profession
	}
	if r.BiggestChallenge != nil {
		dto.BiggestChallenge = *r.BiggestChallenge
	}
	if r.MostUsefulResource != nil {
		dto.MostUsefulRes = *r.MostUsefulResource
	}
	if r.BestAdvice != nil {
		dto.BestAdvice = *r.BestAdvice
	}
	if r.AuthorName != nil {
		dto.AuthorName = *r.AuthorName
	}
	if r.AuthorAvatar != nil {
		dto.AuthorAvatar = *r.AuthorAvatar
	}
	return dto
}

// metaToDTO converts a content row + story_meta into a StoryDTO.
func metaToDTO(c gen.GetContentByIDRow, m StoryMeta) StoryDTO {
	dto := StoryDTO{
		ID:           c.ID.String(),
		Title:        c.Title,
		Slug:         c.Slug,
		Body:         c.Body,
		BodyHTML:     c.BodyHtml,
		Status:       c.Status,
		Visibility:   c.Visibility,
		ViewCount:    c.ViewCount,
		CommentCount: c.CommentCount,
		PublishedAt:  formatTs(c.PublishedAt),
		CreatedAt:    formatTs(c.CreatedAt),
		UpdatedAt:    formatTs(c.UpdatedAt),
		Verified:     m.Verified,
	}
	if m.OriginSpaceID.Valid {
		dto.OriginSpaceID = m.OriginSpaceID.String()
	}
	if m.DestinationSpaceID.Valid {
		dto.DestSpaceID = m.DestinationSpaceID.String()
	}
	if m.RelocationYear.Valid {
		dto.RelocationYear = m.RelocationYear.Int16
	}
	if m.VisaCategory != nil {
		dto.VisaCategory = *m.VisaCategory
	}
	if m.Profession != nil {
		dto.Profession = *m.Profession
	}
	if m.BiggestChallenge != nil {
		dto.BiggestChallenge = *m.BiggestChallenge
	}
	if m.MostUsefulResource != nil {
		dto.MostUsefulRes = *m.MostUsefulResource
	}
	if m.BestAdvice != nil {
		dto.BestAdvice = *m.BestAdvice
	}
	return dto
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}
