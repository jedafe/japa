// Package achievement — service.go orchestrates achievement submission,
// editing, deletion, filtered listing, and admin moderation. It delegates
// content row creation to the content.Service (§14) and manages the
// achievement_meta extension table via the local Store. Feature-flagged via
// achievement.enabled system_setting (§89).
//
// Ref: japaconnect-engineering/sections/89-community-achievement-feed.md
//      japaconnect-engineering/adr/ADR-097-community-achievement-feed.md
package achievement

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

// ContentService is the subset of content.Service the achievement package
// needs. Defined as an interface so the achievement package depends on a
// narrow contract, not the full content service.
type ContentService interface {
	CreateContent(ctx context.Context, tenantID pgtype.UUID, input content.CreateContentInput) (gen.CreateContentRow, error)
	GetContent(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetContentByIDRow, error)
	UpdateContent(ctx context.Context, id, tenantID, editedBy pgtype.UUID, input content.UpdateContentInput, bypassEditWindow bool) (gen.UpdateContentRow, error)
	DeleteContent(ctx context.Context, id, tenantID pgtype.UUID) error
}

// AchievementDTO is the API response shape for a single achievement.
type AchievementDTO struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Slug            string `json:"slug"`
	Body            string `json:"body"`
	BodyHTML        string `json:"body_html"`
	Status          string `json:"status"`
	Visibility      string `json:"visibility"`
	ViewCount       int32  `json:"view_count"`
	CommentCount    int32  `json:"comment_count"`
	PublishedAt     string `json:"published_at"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	MilestoneType   string `json:"milestone_type,omitempty"`
	OriginSpaceID   string `json:"origin_space_id,omitempty"`
	OriginSpaceName string `json:"origin_space_name,omitempty"`
	OriginSpaceCode string `json:"origin_space_code,omitempty"`
	DestSpaceID     string `json:"destination_space_id"`
	DestSpaceName   string `json:"destination_space_name"`
	DestSpaceCode   string `json:"destination_space_code,omitempty"`
	IsAnonymous     bool   `json:"is_anonymous"`
	AuthorID        string `json:"author_id,omitempty"`
	AuthorName      string `json:"author_name,omitempty"`
	AuthorAvatar    string `json:"author_avatar,omitempty"`
}

// SubmitInput is the request body for POST /achievements.
type SubmitInput struct {
	Title              string `json:"title"`
	Body               string `json:"body"`
	MilestoneType      string `json:"milestone_type"`
	DestinationSpaceID string `json:"destination_space_id"`
	OriginSpaceID      string `json:"origin_space_id"`
	IsAnonymous        bool   `json:"is_anonymous"`
}

// EditInput is the request body for PATCH /achievements/:id.
type EditInput struct {
	Title              *string `json:"title"`
	Body               *string `json:"body"`
	MilestoneType      *string `json:"milestone_type"`
	DestinationSpaceID *string `json:"destination_space_id"`
	OriginSpaceID      *string `json:"origin_space_id"`
	IsAnonymous        *bool   `json:"is_anonymous"`
}

// Service holds the business logic for the Community Achievement Feed.
type Service struct {
	store    *Store
	content  ContentService
	settings *config.Settings
}

// NewService creates an achievement service.
func NewService(store *Store, contentSvc ContentService, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		content:  contentSvc,
		settings: settings,
	}
}

// Enabled reads the achievement.enabled system_setting toggle (§89 feature
// flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "achievement.enabled", false)
}

// Submit creates a new achievement — a contents row with
// content_type='achievement' plus an achievement_meta row. The achievement is
// created in 'draft' status; the author can submit for review (set status to
// 'pending') via the content update flow. Auth required (achievement.create
// permission checked at route level).
func (s *Service) Submit(ctx context.Context, tenantID, authorID pgtype.UUID, input SubmitInput) (AchievementDTO, error) {
	if input.Title == "" || input.DestinationSpaceID == "" {
		return AchievementDTO{}, fmt.Errorf("title and destination_space_id are required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return AchievementDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var originSpaceID pgtype.UUID
	if input.OriginSpaceID != "" {
		if err := originSpaceID.Scan(input.OriginSpaceID); err != nil {
			return AchievementDTO{}, fmt.Errorf("invalid origin_space_id: %w", err)
		}
	}

	contentRow, err := s.content.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:    authorID,
		ContentType: content.TypeAchievement,
		Title:       input.Title,
		Body:        input.Body,
		Status:      content.StatusDraft,
		Visibility:  content.VisibilityPublic,
		GeoSpaceID:  destSpaceID, // file achievement in destination country space
	})
	if err != nil {
		return AchievementDTO{}, fmt.Errorf("achievement: create content: %w", err)
	}

	meta := AchievementMeta{
		TenantID:           tenantID,
		ContentID:          contentRow.ID,
		DestinationSpaceID: destSpaceID,
		OriginSpaceID:      originSpaceID,
		IsAnonymous:        input.IsAnonymous,
	}
	if input.MilestoneType != "" {
		meta.MilestoneType = &input.MilestoneType
	}

	if err := s.store.CreateMeta(ctx, meta); err != nil {
		slog.Error("achievement: create achievement_meta failed after content creation", "content_id", contentRow.ID, "error", err)
		return AchievementDTO{}, fmt.Errorf("achievement: create achievement_meta: %w", err)
	}

	return s.getAchievementDTOByID(ctx, contentRow.ID, tenantID)
}

// GetBySlug returns a single published achievement by its slug.
// Anonymous-accessible.
func (s *Service) GetBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (AchievementDTO, error) {
	row, err := s.store.GetBySlug(ctx, slug, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AchievementDTO{}, pgx.ErrNoRows
		}
		return AchievementDTO{}, fmt.Errorf("achievement: get by slug: %w", err)
	}
	return rowToDTO(row), nil
}

// List returns a paginated, filtered list of published achievements.
func (s *Service) List(ctx context.Context, tenantID, destSpaceID pgtype.UUID, milestoneType, sort string, page, limit int32) ([]AchievementDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.ListAchievements(ctx, ListAchievementsFilter{
		TenantID:      tenantID,
		DestSpaceID:   destSpaceID,
		MilestoneType: milestoneType,
		Sort:          sort,
		Status:        content.StatusPublished,
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("achievement: list: %w", err)
	}

	dtos := make([]AchievementDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}
	return dtos, total, nil
}

// ListBySpace returns published achievements filed in the Space identified by
// its slug, for the Space widget.
func (s *Service) ListBySpace(ctx context.Context, spaceSlug string, tenantID pgtype.UUID, limit int32) ([]AchievementDTO, error) {
	rows, err := s.store.ListBySpace(ctx, spaceSlug, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("achievement: list by space: %w", err)
	}
	dtos := make([]AchievementDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}
	return dtos, nil
}

// EditOwn updates the author's own achievement. The caller must verify
// ownership (achievement.edit_own permission + author_id match). Only the
// provided (non-nil) fields are updated.
func (s *Service) EditOwn(ctx context.Context, contentID, tenantID, userID pgtype.UUID, input EditInput) (AchievementDTO, error) {
	// Fetch existing content to verify ownership.
	existing, err := s.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AchievementDTO{}, pgx.ErrNoRows
		}
		return AchievementDTO{}, fmt.Errorf("achievement: edit: get content: %w", err)
	}
	if existing.AuthorID != userID {
		return AchievementDTO{}, ErrNotOwner
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
		return AchievementDTO{}, fmt.Errorf("achievement: edit: update content: %w", err)
	}

	// Update achievement_meta fields.
	meta := AchievementMeta{}
	if input.DestinationSpaceID != nil && *input.DestinationSpaceID != "" {
		if err := meta.DestinationSpaceID.Scan(*input.DestinationSpaceID); err != nil {
			return AchievementDTO{}, fmt.Errorf("achievement: edit: invalid destination_space_id: %w", err)
		}
	}
	if input.OriginSpaceID != nil && *input.OriginSpaceID != "" {
		if err := meta.OriginSpaceID.Scan(*input.OriginSpaceID); err != nil {
			return AchievementDTO{}, fmt.Errorf("achievement: edit: invalid origin_space_id: %w", err)
		}
	}
	if input.MilestoneType != nil {
		meta.MilestoneType = input.MilestoneType
	}
	if input.IsAnonymous != nil {
		meta.IsAnonymous = *input.IsAnonymous
	}

	if err := s.store.UpdateMeta(ctx, contentID, tenantID, meta); err != nil {
		return AchievementDTO{}, fmt.Errorf("achievement: edit: update achievement_meta: %w", err)
	}

	return s.getAchievementDTOByID(ctx, contentID, tenantID)
}

// DeleteOwn soft-deletes the author's own achievement. The caller must verify
// ownership (achievement.delete_own permission + author_id match).
func (s *Service) DeleteOwn(ctx context.Context, contentID, tenantID, userID pgtype.UUID) error {
	existing, err := s.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("achievement: delete: get content: %w", err)
	}
	if existing.AuthorID != userID {
		return ErrNotOwner
	}

	if err := s.content.DeleteContent(ctx, contentID, tenantID); err != nil {
		return fmt.Errorf("achievement: delete: %w", err)
	}
	return nil
}

// AdminList returns all achievements (any status) for the moderation queue.
func (s *Service) AdminList(ctx context.Context, tenantID pgtype.UUID, status, milestoneType, q string, page, limit int32) ([]AchievementDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.AdminList(ctx, AdminListFilter{
		TenantID:      tenantID,
		Status:        status,
		MilestoneType: milestoneType,
		Q:             q,
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("achievement: admin list: %w", err)
	}

	dtos := make([]AchievementDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}
	return dtos, total, nil
}

// getAchievementDTOByID fetches an achievement by content_id and converts to
// DTO.
func (s *Service) getAchievementDTOByID(ctx context.Context, contentID, tenantID pgtype.UUID) (AchievementDTO, error) {
	contentRow, err := s.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		return AchievementDTO{}, fmt.Errorf("achievement: get by id: %w", err)
	}
	meta, err := s.store.GetMeta(ctx, contentID, tenantID)
	if err != nil {
		return AchievementDTO{}, fmt.Errorf("achievement: get meta by id: %w", err)
	}
	return metaToDTO(contentRow, meta), nil
}

// ErrNotOwner is returned when a user tries to edit or delete an achievement
// they did not author.
var ErrNotOwner = fmt.Errorf("not the achievement author")

// rowToDTO converts an AchievementRow (joined query result) to an
// AchievementDTO.
func rowToDTO(r AchievementRow) AchievementDTO {
	dto := AchievementDTO{
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
		IsAnonymous:  r.IsAnonymous,
	}
	if r.MilestoneType != nil {
		dto.MilestoneType = *r.MilestoneType
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
	if !r.IsAnonymous {
		if r.AuthorName != nil {
			dto.AuthorName = *r.AuthorName
		}
		if r.AuthorAvatar != nil {
			dto.AuthorAvatar = *r.AuthorAvatar
		}
	}
	if r.AuthorID.Valid {
		dto.AuthorID = r.AuthorID.String()
	}
	return dto
}

// metaToDTO converts a content row + achievement_meta into an AchievementDTO.
func metaToDTO(c gen.GetContentByIDRow, m AchievementMeta) AchievementDTO {
	dto := AchievementDTO{
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
		IsAnonymous:  m.IsAnonymous,
	}
	if m.MilestoneType != nil {
		dto.MilestoneType = *m.MilestoneType
	}
	if m.OriginSpaceID.Valid {
		dto.OriginSpaceID = m.OriginSpaceID.String()
	}
	if m.DestinationSpaceID.Valid {
		dto.DestSpaceID = m.DestinationSpaceID.String()
	}
	if c.AuthorID.Valid {
		dto.AuthorID = c.AuthorID.String()
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
