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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/config"
)

// AIDispatcher is the contract the service uses to call the §20 AI Gateway
// for Content Agent draft generation. *ai.Dispatcher satisfies this.
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}

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
	ID                 string      `json:"id"`
	DestinationSpaceID string      `json:"destination_space_id"`
	DestSpaceName      string      `json:"destination_space_name,omitempty"`
	DestSpaceCode      string      `json:"destination_space_code,omitempty"`
	VisaCategory       string      `json:"visa_category,omitempty"`
	TestID             pgtype.UUID `json:"test_id"`
	TestSlug           string      `json:"test_slug"`
	TestName           string      `json:"test_name"`
	MinimumScore       string      `json:"minimum_score"`
	ScoreBreakdown     any         `json:"score_breakdown,omitempty"`
	Notes              string      `json:"notes,omitempty"`
	SourceURL          string      `json:"source_url,omitempty"`
	Status             string      `json:"status"`
	IsActive           bool        `json:"is_active"`
	AIGenerated        bool        `json:"ai_generated"`
	AIModel            string      `json:"ai_model,omitempty"`
	AIPromptHash       string      `json:"ai_prompt_hash,omitempty"`
	HITLReviewedBy     string      `json:"hitl_reviewed_by,omitempty"`
	HITLReviewedAt     string      `json:"hitl_reviewed_at,omitempty"`
	HITLReviewNotes    string      `json:"hitl_review_notes,omitempty"`
	CreatedAt          string      `json:"created_at"`
	UpdatedAt          string      `json:"updated_at"`
}

// ResourceDTO is the API response shape for a single prep resource.
type ResourceDTO struct {
	ID              string      `json:"id"`
	TestID          pgtype.UUID `json:"test_id"`
	Title           string      `json:"title"`
	ResourceURL     string      `json:"resource_url"`
	ResourceType    string      `json:"resource_type,omitempty"`
	CommunityRating string      `json:"community_rating,omitempty"`
	RatingCount     int32       `json:"rating_count"`
	CreatedAt       string      `json:"created_at"`
}

// ScoreStoryDTO is the API response shape for a single score story.
type ScoreStoryDTO struct {
	ID                 string      `json:"id"`
	UserID             string      `json:"user_id"`
	TestID             pgtype.UUID `json:"test_id"`
	StartingLevel      string      `json:"starting_level,omitempty"`
	FinalScore         string      `json:"final_score"`
	DestinationSpaceID string      `json:"destination_space_id,omitempty"`
	DestSpaceName      string      `json:"destination_space_name,omitempty"`
	PrepDurationWeeks  int32       `json:"prep_duration_weeks,omitempty"`
	ResourcesUsed      []string    `json:"resources_used,omitempty"`
	Advice             string      `json:"advice,omitempty"`
	Status             string      `json:"status"`
	CreatedAt          string      `json:"created_at"`
	AuthorName         string      `json:"author_name,omitempty"`
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

// UpdateRequirementRequest is the request body for updating an existing
// language test requirement. All fields are optional; only provided
// fields are applied.
type UpdateRequirementRequest struct {
	DestinationSpaceID *string `json:"destination_space_id,omitempty"`
	VisaCategory       *string `json:"visa_category,omitempty"`
	TestSlug           string  `json:"test_slug,omitempty"`
	MinimumScore       *string `json:"minimum_score,omitempty"`
	ScoreBreakdown     any     `json:"score_breakdown,omitempty"`
	Notes              *string `json:"notes,omitempty"`
	SourceURL          *string `json:"source_url,omitempty"`
	Status             *string `json:"status,omitempty"`
	IsActive           *bool   `json:"is_active,omitempty"`
	AIGenerated        *bool   `json:"ai_generated,omitempty"`
	AIModel            *string `json:"ai_model,omitempty"`
	AIPromptHash       *string `json:"ai_prompt_hash,omitempty"`
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
	store      *Store
	settings   *config.Settings
	dispatcher AIDispatcher
}

// NewService creates a language test hub service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// SetDispatcher injects the AI Gateway dispatcher for Content Agent draft
// generation (ADR-088). If unset, GenerateDraft still creates an ai_draft
// row with empty fields for the reviewer to fill manually.
func (s *Service) SetDispatcher(d AIDispatcher) { s.dispatcher = d }

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
		if errors.Is(err, pgx.ErrNoRows) {
			return TestDTO{}, pgx.ErrNoRows
		}

		return TestDTO{}, fmt.Errorf("languagetesthub: get test by slug: %w", err)
	}

	return testRowToDTO(row), nil
}

// GetTestByID returns a single language test by its id.
func (s *Service) GetTestByID(ctx context.Context, id pgtype.UUID) (TestDTO, error) {
	row, err := s.store.GetTestByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TestDTO{}, pgx.ErrNoRows
		}

		return TestDTO{}, fmt.Errorf("languagetesthub: get test by id: %w", err)
	}

	return testRowToDTO(row), nil
}

// SearchTests returns language tests matching the given query, ordered by name.
func (s *Service) SearchTests(ctx context.Context, query string, limit int32) ([]TestDTO, error) {
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	rows, err := s.store.SearchTests(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("languagetesthub: search tests: %w", err)
	}

	dtos := make([]TestDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, testRowToDTO(r))
	}

	return dtos, nil
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
func (s *Service) ListResources(ctx context.Context, tenantID, testID pgtype.UUID, page, limit int32) ([]ResourceDTO, error) {
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
func (s *Service) ListStories(ctx context.Context, tenantID, testID, destSpaceID pgtype.UUID, page, limit int32) ([]ScoreStoryDTO, error) {
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
		return ScoreStoryDTO{}, errors.New("final_score is required")
	}

	test, err := s.store.GetTestBySlug(ctx, testSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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

// CreateLanguageTest creates a new language test. language_tests is a
// global table and ignores the supplied tenant_id.
func (s *Service) CreateLanguageTest(ctx context.Context, tenantID pgtype.UUID, input CreateLanguageTestInput) (TestDTO, error) {
	if input.Slug == "" {
		return TestDTO{}, errors.New("slug is required")
	}

	if input.Name == "" {
		return TestDTO{}, errors.New("name is required")
	}

	row, err := s.store.CreateLanguageTest(ctx, input)
	if err != nil {
		return TestDTO{}, fmt.Errorf("languagetesthub: create language test: %w", err)
	}

	return testRowToDTO(row), nil
}

// UpdateLanguageTest updates a language test by id.
func (s *Service) UpdateLanguageTest(ctx context.Context, id, tenantID pgtype.UUID, input UpdateLanguageTestInput) (TestDTO, error) {
	if input.Slug == nil && input.Name == nil && input.OfficialURL == nil && input.ValidityMonths == nil {
		return TestDTO{}, errors.New("no fields to update")
	}

	row, err := s.store.UpdateLanguageTest(ctx, id, input)
	if err != nil {
		return TestDTO{}, fmt.Errorf("languagetesthub: update language test: %w", err)
	}

	return testRowToDTO(row), nil
}

// DeleteLanguageTest deletes a language test by id.
func (s *Service) DeleteLanguageTest(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.DeleteLanguageTest(ctx, id); err != nil {
		return fmt.Errorf("languagetesthub: delete language test: %w", err)
	}

	return nil
}

// CreateRequirement creates a new requirement (admin). The test is
// resolved by slug from the request.
func (s *Service) CreateRequirement(ctx context.Context, tenantID pgtype.UUID, input CreateRequirementRequest) (RequirementDTO, error) {
	if input.DestinationSpaceID == "" {
		return RequirementDTO{}, errors.New("destination_space_id is required")
	}

	if input.TestSlug == "" {
		return RequirementDTO{}, errors.New("test_slug is required")
	}

	if input.MinimumScore == "" {
		return RequirementDTO{}, errors.New("minimum_score is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return RequirementDTO{}, fmt.Errorf("languagetesthub: create requirement: invalid destination_space_id: %w", err)
	}

	test, err := s.store.GetTestBySlug(ctx, input.TestSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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

// UpdateRequirement updates an existing requirement (admin). The test is
// resolved by slug when provided. Only non-nil request fields are applied.
func (s *Service) UpdateRequirement(ctx context.Context, id, tenantID pgtype.UUID, req UpdateRequirementRequest) (RequirementDTO, error) {
	var input UpdateRequirementInput

	hasField := false

	if req.DestinationSpaceID != nil && *req.DestinationSpaceID != "" {
		if err := input.DestinationSpaceID.Scan(*req.DestinationSpaceID); err != nil {
			return RequirementDTO{}, fmt.Errorf("languagetesthub: update requirement: invalid destination_space_id: %w", err)
		}

		hasField = true
	}

	if req.VisaCategory != nil {
		input.VisaCategory = req.VisaCategory
		hasField = true
	}

	if req.TestSlug != "" {
		test, err := s.store.GetTestBySlug(ctx, req.TestSlug)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return RequirementDTO{}, pgx.ErrNoRows
			}

			return RequirementDTO{}, fmt.Errorf("languagetesthub: update requirement: get test: %w", err)
		}

		input.TestID = test.ID
		hasField = true
	}

	if req.MinimumScore != nil {
		if *req.MinimumScore == "" {
			return RequirementDTO{}, errors.New("languagetesthub: update requirement: minimum_score cannot be empty")
		}

		input.MinimumScore = req.MinimumScore
		hasField = true
	}

	if req.ScoreBreakdown != nil {
		scoreBreakdown, err := marshalJSONB(req.ScoreBreakdown)
		if err != nil {
			return RequirementDTO{}, fmt.Errorf("languagetesthub: update requirement: invalid score_breakdown: %w", err)
		}

		input.ScoreBreakdown = scoreBreakdown
		hasField = true
	}

	if req.Notes != nil {
		input.Notes = req.Notes
		hasField = true
	}

	if req.SourceURL != nil {
		input.SourceURL = req.SourceURL
		hasField = true
	}

	if req.Status != nil {
		input.Status = req.Status
		hasField = true
	}

	if req.IsActive != nil {
		input.IsActive = req.IsActive
		hasField = true
	}

	if req.AIGenerated != nil {
		input.AIGenerated = req.AIGenerated
		hasField = true
	}

	if req.AIModel != nil {
		input.AIModel = req.AIModel
		hasField = true
	}

	if req.AIPromptHash != nil {
		input.AIPromptHash = req.AIPromptHash
		hasField = true
	}

	if !hasField {
		return RequirementDTO{}, errors.New("languagetesthub: update requirement: no fields to update")
	}

	row, err := s.store.UpdateRequirement(ctx, id, tenantID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RequirementDTO{}, pgx.ErrNoRows
		}

		return RequirementDTO{}, fmt.Errorf("languagetesthub: update requirement: %w", err)
	}

	return requirementRowToDTO(row), nil
}

// DeleteRequirement deletes a requirement by id (admin).
func (s *Service) DeleteRequirement(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.DeleteRequirement(ctx, id, tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("languagetesthub: delete requirement: %w", err)
	}

	return nil
}

// GenerateDraft creates an AI-drafted requirement in 'ai_draft' status via
// the §20 AI Gateway (ADR-088 Content Agent draft endpoint pattern). The
// draft is NOT visible to end users and requires HITL approval before
// publication. If the dispatcher is nil or the call fails, the draft is
// still created with empty minimum_score/score_breakdown so the reviewer
// can fill them manually.
func (s *Service) GenerateDraft(ctx context.Context, tenantID pgtype.UUID, input GenerateRequirementRequest) (RequirementDTO, error) {
	if input.DestinationSpaceID == "" {
		return RequirementDTO{}, errors.New("destination_space_id is required")
	}

	if input.TestSlug == "" {
		return RequirementDTO{}, errors.New("test_slug is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return RequirementDTO{}, fmt.Errorf("languagetesthub: generate draft: invalid destination_space_id: %w", err)
	}

	test, err := s.store.GetTestBySlug(ctx, input.TestSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RequirementDTO{}, pgx.ErrNoRows
		}

		return RequirementDTO{}, fmt.Errorf("languagetesthub: generate draft: get test: %w", err)
	}

	prompt := fmt.Sprintf("Generate the minimum %s score requirement for destination %s", test.Name, input.DestinationSpaceID)
	if input.VisaCategory != "" {
		prompt = fmt.Sprintf("Generate the minimum %s score requirement for destination %s, visa category %s", test.Name, input.DestinationSpaceID, input.VisaCategory)
	}

	promptHash := sha256Hex(prompt)

	aiModel := "content_agent_v1"
	minimumScore := ""

	var scoreBreakdown []byte

	var notes *string

	var sourceURL *string

	if s.dispatcher != nil {
		resp, derr := s.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
			Category: "content_generation",
			Payload: map[string]any{
				"prompt":    prompt,
				"tenant_id": tenantID,
			},
		})
		if derr != nil {
			slog.Warn("languagetesthub: AI generation failed, creating draft with empty fields",
				"error", derr)
		} else {
			minimumScore, scoreBreakdown, notes, sourceURL = parseRequirementFromAIResponse(resp)
		}
	}

	var visaCategory *string

	if input.VisaCategory != "" {
		v := input.VisaCategory
		visaCategory = &v
	}

	if scoreBreakdown == nil {
		scoreBreakdown = []byte("null")
	}

	row, err := s.store.CreateRequirement(ctx, CreateRequirementInput{
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		VisaCategory:       visaCategory,
		TestID:             test.ID,
		MinimumScore:       minimumScore,
		ScoreBreakdown:     scoreBreakdown,
		Notes:              notes,
		SourceURL:          sourceURL,
		Status:             StatusAIDraft,
		IsActive:           false, // not active until approved
		AIGenerated:        true,
		AIModel:            &aiModel,
		AIPromptHash:       &promptHash,
	})
	if err != nil {
		return RequirementDTO{}, fmt.Errorf("languagetesthub: generate draft: create: %w", err)
	}

	return requirementRowToDTO(row), nil
}

// sha256Hex returns the hex-encoded SHA-256 of the input (ADR-088 audit
// reproducibility).
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// parseRequirementFromAIResponse extracts the minimum score, score
// breakdown jsonb, notes, and source URL from an AI Gateway response. The
// response may carry structured JSON or plain text; text is placed in
// minimum_score and the raw payload in notes for the reviewer to refine.
func parseRequirementFromAIResponse(resp ai.Response) (string, []byte, *string, *string) {
	minimumScore := ""

	var scoreBreakdown []byte

	var notes *string

	var sourceURL *string

	if text, ok := resp.Raw["text"].(string); ok && text != "" {
		minimumScore = text
	}

	if text, ok := resp.Raw["content"].(string); ok && text != "" && minimumScore == "" {
		minimumScore = text
	}

	if sb, ok := resp.Raw["score_breakdown"]; ok {
		if b, err := json.Marshal(sb); err == nil {
			scoreBreakdown = b
		}
	}

	if n, ok := resp.Raw["notes"].(string); ok && n != "" {
		notes = &n
	}

	if u, ok := resp.Raw["source_url"].(string); ok && u != "" {
		sourceURL = &u
	}

	return minimumScore, scoreBreakdown, notes, sourceURL
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
		if errors.Is(err, pgx.ErrNoRows) {
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
