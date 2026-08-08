// Package languagetesthub — service.go orchestrates the Language Test
// Preparation Hub business logic (§99, ADR-107): listing tests, resolving
// requirements by destination/visa, listing resources and score stories,
// submitting stories, and admin requirement management with HITL review.
// Feature-flagged via language_test_hub.enabled system_setting (§99).
//
// Ref: japaconnect-engineering/sections/99-language-test-hub.md
//
//	japaconnect-engineering/adr/ADR-107-language-test-hub.md
package languagetesthub

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// TestDTO is the API response shape for a single language test.
type TestDTO struct {
	ID             pgtype.UUID `json:"id"`
	Slug           string      `json:"slug"`
	Name           string      `json:"name"`
	OfficialURL    string      `json:"official_url,omitempty"`
	ValidityMonths int32       `json:"validity_months,omitempty"`
}

// RequirementDTO is the API response shape for a single requirement.
type RequirementDTO struct {
	ID                 string `json:"id"`
	DestinationSpaceID string `json:"destination_space_id"`
	DestSpaceName      string `json:"destination_space_name,omitempty"`
	DestSpaceCode      string `json:"destination_space_code,omitempty"`
	VisaCategory       string `json:"visa_category,omitempty"`
	TestID             pgtype.UUID `json:"test_id"`
	TestSlug           string      `json:"test_slug"`
	TestName           string      `json:"test_name"`
	MinimumScore       string      `json:"minimum_score"`
	ScoreBreakdown     any    `json:"score_breakdown,omitempty"`
	Notes              string `json:"notes,omitempty"`
	SourceURL          string `json:"source_url,omitempty"`
	Status             string `json:"status"`
	IsActive           bool   `json:"is_active"`
	AIGenerated        bool   `json:"ai_generated"`
	AIModel            string `json:"ai_model,omitempty"`
	AIPromptHash       string `json:"ai_prompt_hash,omitempty"`
	HITLReviewedBy     string `json:"hitl_reviewed_by,omitempty"`
	HITLReviewedAt     string `json:"hitl_reviewed_at,omitempty"`
	HITLReviewNotes    string `json:"hitl_review_notes,omitempty"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

// ResourceDTO is the API response shape for a single prep resource.
type ResourceDTO struct {
	ID              string      `json:"id"`
	TestID          pgtype.UUID `json:"test_id"`
	Title           string      `json:"title"`
	ResourceURL     string `json:"resource_url"`
	ResourceType    string `json:"resource_type,omitempty"`
	CommunityRating string `json:"community_rating,omitempty"`
	RatingCount     int32  `json:"rating_count"`
	CreatedAt       string `json:"created_at"`
}

// ScoreStoryDTO is the API response shape for a single score story.
type ScoreStoryDTO struct {
	ID                 string      `json:"id"`
	UserID             string      `json:"user_id"`
	TestID             pgtype.UUID `json:"test_id"`
	StartingLevel      string      `json:"starting_level,omitempty"`
	FinalScore         string   `json:"final_score"`
	DestinationSpaceID string   `json:"destination_space_id,omitempty"`
	DestSpaceName      string   `json:"destination_space_name,omitempty"`
	PrepDurationWeeks  int32    `json:"prep_duration_weeks,omitempty"`
	ResourcesUsed      []string `json:"resources_used,omitempty"`
	Advice             string   `json:"advice,omitempty"`
	Status             string   `json:"status"`
	CreatedAt          string   `json:"created_at"`
	AuthorName         string   `json:"author_name,omitempty"`
}

// CreateRequirementRequest is the request body for POST
// /admin/language-tests/requirements.
type CreateRequirementRequest struct {
	DestinationSpaceID string `json:"destination_space_id"`
	VisaCategory       string `json:"visa_category"`
	TestSlug           string `json:"test_slug"`
	MinimumScore       string `json:"minimum_score"`
	ScoreBreakdown     any    `json:"score_breakdown"`
	Notes              string `json:"notes"`
	SourceURL          string `json:"source_url"`
	Status             string `json:"status"`
	IsActive           *bool  `json:"is_active"`
	AIGenerated        bool   `json:"ai_generated"`
	AIModel            string `json:"ai_model"`
	AIPromptHash       string `json:"ai_prompt_hash"`
}

// CreateStoryRequest is the request body for POST
// /language-tests/:test_slug/stories.
type CreateStoryRequest struct {
	StartingLevel      string   `json:"starting_level"`
	FinalScore         string   `json:"final_score"`
	DestinationSpaceID string   `json:"destination_space_id"`
	PrepDurationWeeks  int32    `json:"prep_duration_weeks"`
	ResourcesUsed      []string `json:"resources_used"`
	Advice             string   `json:"advice"`
}

// GenerateRequirementRequest is the request body for POST
// /admin/language-tests/requirements/generate (Content Agent draft
// placeholder).
type GenerateRequirementRequest struct {
	DestinationSpaceID string `json:"destination_space_id"`
	VisaCategory       string `json:"visa_category"`
	TestSlug           string `json:"test_slug"`
}

// ApproveRequirementRequest is the request body for PATCH
// /admin/language-tests/requirements/:id/approve.
type ApproveRequirementRequest struct {
	ReviewNotes string `json:"review_notes"`
}

// Service holds the business logic for the Language Test Preparation Hub.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates a language test hub service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// Enabled reads the language_test_hub.enabled system_setting toggle
// (§99 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "language_test_hub.enabled", false)
}

// ListTests returns all language tests.
func (s *Service) ListTests(ctx context.Context) ([]TestDTO, error) {
	rows, err := s.store.ListTests(ctx)
	if err != nil {
		return nil, fmt.Errorf("languagetesthub: list tests: %w", err)
	}
	dtos := make([]TestDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, testRowToDTO(r))
	}
	return dtos, nil
}

// GetTestBySlug returns a single language test by its slug.
func (s *Service) GetTestBySlug(ctx context.Context, slug string) (TestDTO, error) {
	row, err := s.store.GetTestBySlug(ctx, slug)
	if err != nil {
		if err == pgx.ErrNoRows {
			return TestDTO{}, pgx.ErrNoRows
		}
		return TestDTO{}, fmt.Errorf("languagetesthub: get test by slug: %w", err)
	}
	return testRowToDTO(row), nil
}

// ListRequirements returns published, active requirements for the public
// requirements endpoint, optionally filtered by destination space and
// visa category.
func (s *Service) ListRequirements(ctx context.Context, tenantID, destSpaceID pgtype.UUID, visaCategory string) ([]RequirementDTO, error) {
	rows, err := s.store.ListRequirements(ctx, ListRequirementsFilter{
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		VisaCategory:       visaCategory,
	})
	if err != nil {
		return nil, fmt.Errorf("languagetesthub: list requirements: %w", err)
	}
	dtos := make([]RequirementDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, requirementRowToDTO(r))
	}
	return dtos, nil
}

// ListResources returns prep resources for a test.
func (s *Service) ListResources(ctx context.Context, tenantID pgtype.UUID, testID pgtype.UUID, page, limit int32) ([]ResourceDTO, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, err := s.store.ListResources(ctx, tenantID, testID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("languagetesthub: list resources: %w", err)
	}
	dtos := make([]ResourceDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, resourceRowToDTO(r))
	}
	return dtos, nil
}

// ListStories returns published score stories for a test, optionally
// filtered by destination space.
func (s *Service) ListStories(ctx context.Context, tenantID pgtype.UUID, testID pgtype.UUID, destSpaceID pgtype.UUID, page, limit int32) ([]ScoreStoryDTO, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, err := s.store.ListStories(ctx, tenantID, testID, destSpaceID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("languagetesthub: list stories: %w", err)
	}
	dtos := make([]ScoreStoryDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, storyRowToDTO(r))
	}
	return dtos, nil
}

// CreateStory submits a new score story for the given test slug on behalf
// of the authenticated user.
func (s *Service) CreateStory(ctx context.Context, tenantID, userID pgtype.UUID, testSlug string, input CreateStoryRequest) (ScoreStoryDTO, error) {
	if input.FinalScore == "" {
		return ScoreStoryDTO{}, fmt.Errorf("final_score is required")
	}

	test, err := s.store.GetTestBySlug(ctx, testSlug)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ScoreStoryDTO{}, pgx.ErrNoRows
		}
		return ScoreStoryDTO{}, fmt.Errorf("languagetesthub: create story: get test: %w", err)
	}

	var destSpaceID pgtype.UUID
	if input.DestinationSpaceID != "" {
		if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
			return ScoreStoryDTO{}, fmt.Errorf("languagetesthub: create story: invalid destination_space_id: %w", err)
		}
	}

	var startingLevel *string
	if input.StartingLevel != "" {
		s := input.StartingLevel
		startingLevel = &s
	}
	var prepWeeks *int32
	if input.PrepDurationWeeks > 0 {
		w := input.PrepDurationWeeks
		prepWeeks = &w
	}
	var advice *string
	if input.Advice != "" {
		a := input.Advice
		advice = &a
	}

	row, err := s.store.CreateStory(ctx, CreateStoryInput{
		TenantID:           tenantID,
		UserID:             userID,
		TestID:             test.ID,
		StartingLevel:      startingLevel,
		FinalScore:         input.FinalScore,
		DestinationSpaceID: destSpaceID,
		PrepDurationWeeks:  prepWeeks,
		ResourcesUsed:      input.ResourcesUsed,
		Advice:             advice,
	})
	if err != nil {
		return ScoreStoryDTO{}, fmt.Errorf("languagetesthub: create story: %w", err)
	}
	return storyRowToDTO(row), nil
}

// CreateRequirement creates a new requirement (admin). The test is
// resolved by slug from the request.
func (s *Service) CreateRequirement(ctx context.Context, tenantID pgtype.UUID, input CreateRequirementRequest) (RequirementDTO, error) {
	if input.DestinationSpaceID == "" {
		return RequirementDTO{}, fmt.Errorf("destination_space_id is required")
	}
	if input.TestSlug == "" {
		return RequirementDTO{}, fmt.Errorf("test_slug is required")
	}
	if input.MinimumScore == "" {
		return RequirementDTO{}, fmt.Errorf("minimum_score is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return RequirementDTO{}, fmt.Errorf("languagetesthub: create requirement: invalid destination_space_id: %w", err)
	}

	test, err := s.store.GetTestBySlug(ctx, input.TestSlug)
	if err != nil {
		if err == pgx.ErrNoRows {
			return RequirementDTO{}, pgx.ErrNoRows
		}
		return RequirementDTO{}, fmt.Errorf("languagetesthub: create requirement: get test: %w", err)
	}

	var visaCategory *string
	if input.VisaCategory != "" {
		v := input.VisaCategory
		visaCategory = &v
	}
	var notes *string
	if input.Notes != "" {
		n := input.Notes
		notes = &n
	}
	var sourceURL *string
	if input.SourceURL != "" {
		u := input.SourceURL
		sourceURL = &u
	}
	var aiModel *string
	if input.AIModel != "" {
		m := input.AIModel
		aiModel = &m
	}
	var aiPromptHash *string
	if input.AIPromptHash != "" {
		h := input.AIPromptHash
		aiPromptHash = &h
	}

	scoreBreakdown, err := marshalJSONB(input.ScoreBreakdown)
	if err != nil {
		return RequirementDTO{}, fmt.Errorf("languagetesthub: create requirement: invalid score_breakdown: %w", err)
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	row, err := s.store.CreateRequirement(ctx, CreateRequirementInput{
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		VisaCategory:       visaCategory,
		TestID:             test.ID,
		MinimumScore:       input.MinimumScore,
		ScoreBreakdown:     scoreBreakdown,
		Notes:              notes,
		SourceURL:          sourceURL,
		Status:             input.Status,
		IsActive:           isActive,
		AIGenerated:        input.AIGenerated,
		AIModel:            aiModel,
		AIPromptHash:       aiPromptHash,
	})
	if err != nil {
		return RequirementDTO{}, fmt.Errorf("languagetesthub: create requirement: %w", err)
	}
	return requirementRowToDTO(row), nil
}

// ApproveRequirement marks a requirement as reviewed and published by an
// admin (HITL approve).
func (s *Service) ApproveRequirement(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, reviewNotes string) error {
	var notes *string
	if reviewNotes != "" {
		n := reviewNotes
		notes = &n
	}
	if err := s.store.ApproveRequirement(ctx, id, tenantID, reviewerID, notes); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("languagetesthub: approve requirement: %w", err)
	}
	return nil
}

// AdminListRequirements returns all requirements (any status) for the
// admin moderation queue.
func (s *Service) AdminListRequirements(ctx context.Context, tenantID, destSpaceID pgtype.UUID, status, q string, page, limit int32) ([]RequirementDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.AdminListRequirements(ctx, AdminListRequirementsFilter{
		TenantID:           tenantID,
		Status:             status,
		DestinationSpaceID: destSpaceID,
		Q:                  q,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("languagetesthub: admin list requirements: %w", err)
	}
	dtos := make([]RequirementDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, requirementRowToDTO(r))
	}
	return dtos, total, nil
}

// ---------------------------------------------------------------------------
// DTO converters
// ---------------------------------------------------------------------------

func testRowToDTO(r TestRow) TestDTO {
	dto := TestDTO{
		ID:   r.ID,
		Slug: r.Slug,
		Name: r.Name,
	}
	if r.OfficialURL != nil {
		dto.OfficialURL = *r.OfficialURL
	}
	if r.ValidityMonths != nil {
		dto.ValidityMonths = *r.ValidityMonths
	}
	return dto
}

func requirementRowToDTO(r RequirementRow) RequirementDTO {
	dto := RequirementDTO{
		TestID:       r.TestID,
		TestSlug:     r.TestSlug,
		TestName:     r.TestName,
		MinimumScore: r.MinimumScore,
		Status:       r.Status,
		IsActive:     r.IsActive,
		AIGenerated:  r.AIGenerated,
		CreatedAt:    formatTs(r.CreatedAt),
		UpdatedAt:    formatTs(r.UpdatedAt),
	}
	if r.ID.Valid {
		dto.ID = r.ID.String()
	}
	if r.DestinationSpaceID.Valid {
		dto.DestinationSpaceID = r.DestinationSpaceID.String()
	}
	if r.DestSpaceName != nil {
		dto.DestSpaceName = *r.DestSpaceName
	}
	if r.DestSpaceCode != nil {
		dto.DestSpaceCode = *r.DestSpaceCode
	}
	if r.VisaCategory != nil {
		dto.VisaCategory = *r.VisaCategory
	}
	dto.ScoreBreakdown = parseJSONB(r.ScoreBreakdown)
	if r.Notes != nil {
		dto.Notes = *r.Notes
	}
	if r.SourceURL != nil {
		dto.SourceURL = *r.SourceURL
	}
	if r.AIModel != nil {
		dto.AIModel = *r.AIModel
	}
	if r.AIPromptHash != nil {
		dto.AIPromptHash = *r.AIPromptHash
	}
	if r.HITLReviewedBy.Valid {
		dto.HITLReviewedBy = r.HITLReviewedBy.String()
	}
	dto.HITLReviewedAt = formatTs(r.HITLReviewedAt)
	if r.HITLReviewNotes != nil {
		dto.HITLReviewNotes = *r.HITLReviewNotes
	}
	return dto
}

func resourceRowToDTO(r ResourceRow) ResourceDTO {
	dto := ResourceDTO{
		TestID:      r.TestID,
		Title:       r.Title,
		ResourceURL: r.ResourceURL,
		RatingCount: r.RatingCount,
		CreatedAt:   formatTs(r.CreatedAt),
	}
	if r.ID.Valid {
		dto.ID = r.ID.String()
	}
	if r.ResourceType != nil {
		dto.ResourceType = *r.ResourceType
	}
	if r.CommunityRating != nil {
		dto.CommunityRating = *r.CommunityRating
	}
	return dto
}

func storyRowToDTO(r ScoreStoryRow) ScoreStoryDTO {
	dto := ScoreStoryDTO{
		TestID:     r.TestID,
		FinalScore: r.FinalScore,
		Status:     r.Status,
		CreatedAt:  formatTs(r.CreatedAt),
	}
	if r.ID.Valid {
		dto.ID = r.ID.String()
	}
	if r.UserID.Valid {
		dto.UserID = r.UserID.String()
	}
	if r.StartingLevel != nil {
		dto.StartingLevel = *r.StartingLevel
	}
	if r.DestinationSpaceID.Valid {
		dto.DestinationSpaceID = r.DestinationSpaceID.String()
	}
	if r.DestSpaceName != nil {
		dto.DestSpaceName = *r.DestSpaceName
	}
	if r.PrepDurationWeeks != nil {
		dto.PrepDurationWeeks = *r.PrepDurationWeeks
	}
	if r.ResourcesUsed != nil {
		dto.ResourcesUsed = r.ResourcesUsed
	}
	if r.Advice != nil {
		dto.Advice = *r.Advice
	}
	if r.AuthorName != nil {
		dto.AuthorName = *r.AuthorName
	}
	return dto
}

// marshalJSONB converts the input (any) into a JSON []byte for jsonb
// storage. nil/empty values produce JSON null.
func marshalJSONB(v any) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// parseJSONB decodes the JSON []byte into a generic value for the DTO.
// Invalid/empty payloads yield nil.
func parseJSONB(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	return v
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}
