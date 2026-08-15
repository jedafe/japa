// Package story — service.go orchestrates story submission, editing,
// deletion, filtered listing, and admin verification. It delegates content
// row creation to the content.Service (§14) and manages the story_meta
// extension table via the local Store. Feature-flagged via story.enabled
// system_setting (§79).
//
// Ref: japaconnect-engineering/sections/79-community-story-wall.md
//
//	japaconnect-engineering/adr/ADR-087-community-story-wall.md
package story

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/ai"
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

// FeatureChecker checks whether a feature flag is enabled for a tenant.
// Satisfied by *features.Checker via structural typing — defined locally to
// avoid importing the features package (keeps the dependency graph clean).
type FeatureChecker interface {
	IsEnabled(ctx context.Context, tenantID pgtype.UUID, key string) bool
}

// StoryDTO is the API response shape for a single story.
type StoryDTO struct {
	ID               string `json:"id"`
	AuthorID         string `json:"author_id,omitempty"`
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
	store      *Store
	content    ContentService
	settings   *config.Settings
	checker    FeatureChecker
	dispatcher AIDispatcher
}

// AIDispatcher is the subset of the AI Gateway the story service needs to
// generate draft stories (content_generation category, ADR-088).
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}

// NewService creates a story service.
func NewService(store *Store, contentSvc ContentService, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		content:  contentSvc,
		settings: settings,
	}
}

// SetFeatureChecker injects the feature flag checker so Enabled can read the
// feature_flags table (the single source of truth for feature reachability)
// instead of system_settings. Falls back to system_settings when nil.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// SetDispatcher injects the AI Gateway dispatcher for draft generation
// (ADR-088). Optional: when nil, GenerateDraft falls back to the prompt
// as the story body.
func (s *Service) SetDispatcher(d AIDispatcher) { s.dispatcher = d }

// Enabled checks whether the story feature is enabled for the given tenant.
// Delegates to the features.Checker (feature_flags table) when available,
// falling back to system_settings for backward compatibility.
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, tenantID, "story.enabled")
	}
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "story.enabled", false)
}

// Submit creates a new story — a contents row with content_type='story' plus
// a story_meta row. The story is created in 'pending' status so it enters the
// moderation queue immediately per §79 ("Submission requires moderation
// approval before publication").
// Auth required (story.create permission checked at route level).
func (s *Service) Submit(ctx context.Context, tenantID, authorID pgtype.UUID, input SubmitStoryInput) (StoryDTO, error) {
	if input.Title == "" || input.DestinationSpaceID == "" {
		return StoryDTO{}, fmt.Errorf("%w: title and destination_space_id are required", ErrValidation)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return StoryDTO{}, fmt.Errorf("%w: invalid destination_space_id", ErrValidation)
	}

	var originSpaceID pgtype.UUID
	if input.OriginSpaceID != "" {
		if err := originSpaceID.Scan(input.OriginSpaceID); err != nil {
			return StoryDTO{}, fmt.Errorf("%w: invalid origin_space_id", ErrValidation)
		}
	}

	contentRow, err := s.content.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:    authorID,
		ContentType: content.TypeStory,
		Title:       input.Title,
		Body:        input.Body,
		Status:      content.StatusInReview, // submit directly into moderation queue per §79
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
// pageSize and pageOffset follow the conventions/api-response-format.md
// offset pagination convention (?page_size=N&page_offset=N).
func (s *Service) List(ctx context.Context, tenantID pgtype.UUID, originSpaceID, destSpaceID pgtype.UUID, visaCategory, profession, sort string, pageSize, pageOffset int32) ([]StoryDTO, int64, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageOffset < 0 {
		pageOffset = 0
	}

	rows, total, err := s.store.ListStories(ctx, ListStoriesFilter{
		TenantID:      tenantID,
		OriginSpaceID: originSpaceID,
		DestSpaceID:   destSpaceID,
		VisaCategory:  visaCategory,
		Profession:    profession,
		Sort:          sort,
		Status:        content.StatusPublished,
		Limit:         pageSize,
		Offset:        pageOffset,
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
// fields are updated. If bypassOwnership is true (caller has story.edit_any),
// the ownership check is skipped so moderators/admins can edit any story.
func (s *Service) EditOwn(ctx context.Context, contentID, tenantID, userID pgtype.UUID, input EditStoryInput, bypassOwnership bool) (StoryDTO, error) {
	// Fetch existing content to verify ownership.
	existing, err := s.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return StoryDTO{}, pgx.ErrNoRows
		}
		return StoryDTO{}, fmt.Errorf("story: edit: get content: %w", err)
	}
	if !bypassOwnership && existing.AuthorID != userID {
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
			return StoryDTO{}, fmt.Errorf("%w: invalid origin_space_id", ErrValidation)
		}
	}
	if input.DestinationSpaceID != nil && *input.DestinationSpaceID != "" {
		if err := meta.DestinationSpaceID.Scan(*input.DestinationSpaceID); err != nil {
			return StoryDTO{}, fmt.Errorf("%w: invalid destination_space_id", ErrValidation)
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
// ownership (story.delete_own permission + author_id match). If
// bypassOwnership is true (caller has story.delete_any), the ownership check
// is skipped so moderators/admins can delete any story.
func (s *Service) DeleteOwn(ctx context.Context, contentID, tenantID, userID pgtype.UUID, bypassOwnership bool) error {
	existing, err := s.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("story: delete: get content: %w", err)
	}
	if !bypassOwnership && existing.AuthorID != userID {
		return ErrNotOwner
	}

	if err := s.content.DeleteContent(ctx, contentID, tenantID); err != nil {
		return fmt.Errorf("story: delete: %w", err)
	}
	return nil
}

// Verify marks a story as verified by an admin (story.verify permission).
// Sets hitl_reviewed_by and hitl_reviewed_at per ADR-088 HITL review audit
// trail.
func (s *Service) Verify(ctx context.Context, contentID, tenantID, adminUserID pgtype.UUID) error {
	if err := s.store.VerifyStory(ctx, contentID, tenantID, adminUserID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("story: verify: %w", err)
	}
	return nil
}

// Unverify marks a story as unverified by an admin (story.verify permission).
// Clears the verified flag and the HITL review audit trail.
func (s *Service) Unverify(ctx context.Context, contentID, tenantID pgtype.UUID) error {
	if err := s.store.UnverifyStory(ctx, contentID, tenantID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("story: unverify: %w", err)
	}
	return nil
}

// AdminList returns all stories (any status) for the moderation queue.
// pageSize and pageOffset follow the conventions/api-response-format.md
// offset pagination convention. sort may be "latest" (default), "oldest",
// or "title". destSpaceID optionally filters by destination country.
func (s *Service) AdminList(ctx context.Context, tenantID pgtype.UUID, status, q string, verified *bool, sort string, destSpaceID pgtype.UUID, pageSize, pageOffset int32) ([]StoryDTO, int64, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageOffset < 0 {
		pageOffset = 0
	}

	rows, total, err := s.store.AdminListStories(ctx, AdminListStoriesFilter{
		TenantID:   tenantID,
		Status:     status,
		Verified:   verified,
		Q:          q,
		Sort:       sort,
		DestSpaceID: destSpaceID,
		Limit:      pageSize,
		Offset:     pageOffset,
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
// Uses the joined GetStoryByID query so origin/destination space names and
// author info are populated.
func (s *Service) getStoryDTOByID(ctx context.Context, contentID, tenantID pgtype.UUID) (StoryDTO, error) {
	row, err := s.store.GetStoryByID(ctx, contentID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return StoryDTO{}, pgx.ErrNoRows
		}
		return StoryDTO{}, fmt.Errorf("story: get by id: %w", err)
	}
	return rowToDTO(row), nil
}

// ErrValidation is returned for client-side validation failures (missing or
// malformed input). Handlers map this to HTTP 400.
var ErrValidation = fmt.Errorf("validation error")

// ErrNotOwner is returned when a user tries to edit or delete a story they
// did not author.
var ErrNotOwner = fmt.Errorf("not the story author")

// rowToDTO converts a StoryRow (joined query result) to a StoryDTO.
func rowToDTO(r StoryRow) StoryDTO {
	dto := StoryDTO{
		ID:           r.ID.String(),
		AuthorID:     r.AuthorID.String(),
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

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// GenerateDraftRequest is the input for AI-generated story drafts (ADR-088,
// routed from the Content Studio).
type GenerateDraftRequest struct {
	DestinationSpaceID string `json:"destination_space_id"`
	VisaCategory       string `json:"visa_category"`
	Profession         string `json:"profession"`
	Prompt             string `json:"prompt"`
}

// GenerateDraftResponse is the response from Content Agent draft generation.
type GenerateDraftResponse struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	ContentID  string `json:"content_id"`
	StoryTitle string `json:"story_title"`
}

// GenerateDraft creates a story draft via the Content Agent (§79, ADR-088).
// It creates a contents row with content_type='story' (status='draft') and a
// story_meta row with ai_generated=true, ai_model='content_agent_v1'. The
// draft requires HITL verification before publication.
//
// When an AI dispatcher is wired (SetDispatcher), the method calls the AI
// Gateway via the "content_generation" category. If the dispatcher is nil,
// the draft is still created with the prompt as the body so a reviewer can
// fill it manually (graceful degradation).
func (s *Service) GenerateDraft(ctx context.Context, tenantID, authorID pgtype.UUID, input GenerateDraftRequest) (GenerateDraftResponse, error) {
	if input.DestinationSpaceID == "" {
		return GenerateDraftResponse{}, fmt.Errorf("destination_space_id is required")
	}
	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return GenerateDraftResponse{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	// Build the prompt sent to the Content Agent.
	prompt := input.Prompt
	if prompt == "" {
		prompt = fmt.Sprintf(
			"Generate a community story about relocating to destination_space_id=%s, visa_category=%s, profession=%s. Include the biggest challenge, most useful resource, and best advice.",
			input.DestinationSpaceID, input.VisaCategory, input.Profession,
		)
	}

	// ai_prompt_hash = SHA256(prompt) for audit traceability.
	sum := sha256.Sum256([]byte(prompt))
	promptHash := hex.EncodeToString(sum[:])

	// Call the AI Gateway when a dispatcher is wired.
	body := prompt
	storyTitle := fmt.Sprintf("AI Draft: %s", input.VisaCategory)
	if input.Profession != "" {
		storyTitle = fmt.Sprintf("AI Draft: %s (%s)", input.Profession, input.VisaCategory)
	}
	if s.dispatcher != nil {
		resp, aiErr := s.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
			Category: "content_generation",
			Payload: map[string]any{
				"prompt":     prompt,
				"tenant_id":  tenantID,
				"guide_type": "story",
			},
		})
		if aiErr != nil {
			return GenerateDraftResponse{}, fmt.Errorf("story: AI generation failed: %w", aiErr)
		}
		if text, ok := resp.Raw["text"].(string); ok {
			body = text
		} else if text, ok := resp.Raw["content"].(string); ok {
			body = text
		} else if text, ok := resp.Raw["output"].(string); ok {
			body = text
		}
		slog.Info("story: AI Gateway generated draft",
			"destination_space_id", input.DestinationSpaceID,
			"visa_category", input.VisaCategory, "prompt_hash", promptHash)
	}

	// 1. Create the contents row with content_type='story', status='draft'.
	contentRow, err := s.content.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:    authorID,
		ContentType: content.TypeStory,
		Title:       storyTitle,
		Body:        body,
		Status:      content.StatusDraft,
		Visibility:  content.VisibilityPublic,
		GeoSpaceID:  destSpaceID,
	})
	if err != nil {
		return GenerateDraftResponse{}, fmt.Errorf("story: generate draft: create content: %w", err)
	}

	// 2. Create the story_meta row with AI provenance fields.
	aiModel := "content_agent_v1"
	meta := StoryMeta{
		TenantID:           tenantID,
		ContentID:          contentRow.ID,
		DestinationSpaceID: destSpaceID,
		Verified:           false,
		AiGenerated:        true,
		AiModel:            &aiModel,
	}
	if input.VisaCategory != "" {
		meta.VisaCategory = &input.VisaCategory
	}
	if input.Profession != "" {
		meta.Profession = &input.Profession
	}
	if err := s.store.CreateStoryMeta(ctx, meta); err != nil {
		slog.Error("story: generate draft: create story_meta failed",
			"content_id", contentRow.ID, "error", err)
		return GenerateDraftResponse{}, fmt.Errorf("story: generate draft: create story_meta: %w", err)
	}

	return GenerateDraftResponse{
		Status:     "accepted",
		Message:    "Content Agent draft created in 'draft' status with story_meta. Review and verify to publish.",
		ContentID:  contentRow.ID.String(),
		StoryTitle: storyTitle,
	}, nil
}
