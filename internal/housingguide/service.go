// Package housingguide — service.go orchestrates housing guide listing,
// retrieval, admin CRUD, Content Agent draft generation, and HITL
// approval. Feature-flagged via housing_guide.enabled system_setting
// (§96, ADR-104).
//
// Ref: japaconnect-engineering/sections/96-housing-guide.md
//
//	japaconnect-engineering/adr/ADR-104-housing-guide.md
package housingguide

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/config"
)

// GuideDTO is the API response shape for a single housing guide.
type GuideDTO struct {
	ID                   string          `json:"id"`
	DestinationSpaceID   string          `json:"destination_space_id"`
	CitySpaceID          string          `json:"city_space_id,omitempty"`
	AccommodationType    string          `json:"accommodation_type,omitempty"`
	SearchPlatforms      json.RawMessage `json:"search_platforms,omitempty"`
	LandlordRequirements string          `json:"landlord_requirements,omitempty"`
	RentRanges           json.RawMessage `json:"rent_ranges,omitempty"`
	ScamCategories       []string        `json:"scam_categories,omitempty"`
	Tips                 string          `json:"tips,omitempty"`
	Status               string          `json:"status"`
	IsActive             bool            `json:"is_active"`
	AIGenerated          bool            `json:"ai_generated"`
	AIModel              string          `json:"ai_model,omitempty"`
	AIPromptHash         string          `json:"ai_prompt_hash,omitempty"`
	HITLReviewedBy       string          `json:"hitl_reviewed_by,omitempty"`
	HITLReviewedAt       string          `json:"hitl_reviewed_at,omitempty"`
	HITLReviewNotes      string          `json:"hitl_review_notes,omitempty"`
	LastVerifiedAt       string          `json:"last_verified_at,omitempty"`
	IsJapabotIndexed     bool            `json:"is_japabot_indexed"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
	DestSpaceName        string          `json:"destination_space_name,omitempty"`
	DestSpaceCode        string          `json:"destination_space_code,omitempty"`
	CitySpaceName        string          `json:"city_space_name,omitempty"`
	CitySpaceCode        string          `json:"city_space_code,omitempty"`
	ReviewerName         string          `json:"reviewer_name,omitempty"`
}

// CreateGuideRequest is the request body for POST /admin/housing-guides.
type CreateGuideRequest struct {
	DestinationSpaceID   string          `json:"destination_space_id"`
	CitySpaceID          string          `json:"city_space_id"`
	AccommodationType    string          `json:"accommodation_type"`
	SearchPlatforms      json.RawMessage `json:"search_platforms"`
	LandlordRequirements string          `json:"landlord_requirements"`
	RentRanges           json.RawMessage `json:"rent_ranges"`
	ScamCategories       []string        `json:"scam_categories"`
	Tips                 string          `json:"tips"`
	Status               string          `json:"status"`
	IsActive             *bool           `json:"is_active"`
	AIModel              string          `json:"ai_model"`
	AIPromptHash         string          `json:"ai_prompt_hash"`
}

// UpdateGuideRequest is the request body for PATCH /admin/housing-guides/:id.
// All fields are optional; only provided fields are applied.
type UpdateGuideRequest struct {
	AccommodationType    *string         `json:"accommodation_type"`
	SearchPlatforms      json.RawMessage `json:"search_platforms"`
	LandlordRequirements *string         `json:"landlord_requirements"`
	RentRanges           json.RawMessage `json:"rent_ranges"`
	ScamCategories       []string        `json:"scam_categories"`
	Tips                 *string         `json:"tips"`
	Status               *string         `json:"status"`
	IsActive             *bool           `json:"is_active"`
	AIModel              *string         `json:"ai_model"`
	AIPromptHash         *string         `json:"ai_prompt_hash"`
	IsJapabotIndexed     *bool           `json:"is_japabot_indexed"`
}

// ApproveGuideRequest is the request body for PATCH
// /admin/housing-guides/:id/approve.
type ApproveGuideRequest struct {
	Notes *string `json:"notes"`
}

// GenerateDraftResponse is the response from Content Agent draft generation.
type GenerateDraftResponse struct {
	Status             string `json:"status"`
	Message            string `json:"message"`
	GuideID            string `json:"guide_id"`
	DestinationSpaceID string `json:"destination_space_id"`
}

// GenerateGuideRequest is the request body for POST
// /admin/housing-guides/generate (Content Agent draft placeholder).
type GenerateGuideRequest struct {
	DestinationSpaceID string `json:"destination_space_id"`
	CitySpaceID        string `json:"city_space_id"`
	AccommodationType  string `json:"accommodation_type"`
}

// Enqueuer — Service holds the business logic for the Housing & Accommodation Finder
// Guide.
// Enqueuer is the subset of worker.Enqueuer that housingguide needs for
// fire-and-forget task dispatch (Rule 8).
type Enqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// AIDispatcher is the subset of the AI Gateway the housing guide service
// needs to generate draft guides (content_generation category). *ai.Dispatcher
// satisfies this.
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}

// Researcher runs grounded web research for a destination/city and returns
// a formatted research section plus a list of citation URLs. *Service
// implements this interface, but it can be swapped in tests.
type Researcher interface {
	Research(ctx context.Context, tenantID, destSpaceID, citySpaceID pgtype.UUID, accommodationType string) (string, []string, error)
}

// Service is an exported type.
type Service struct {
	store      *Store
	settings   *config.Settings
	enqueuer   Enqueuer     // nil = no task dispatch
	dispatcher AIDispatcher // nil = no AI generation
	httpClient *http.Client // nil = http.DefaultClient
	researcher Researcher   // nil = use s.Research
}

// NewService creates a housing guide service.
func NewService(store *Store, settings *config.Settings) *Service {
	s := &Service{
		store:    store,
		settings: settings,
	}
	s.researcher = s

	return s
}

// SetEnqueuer injects the Asynq task enqueuer for fire-and-forget dispatch
// (Rule 8). Optional — when nil, task dispatch is skipped.
func (s *Service) SetEnqueuer(e Enqueuer) { s.enqueuer = e }

// SetDispatcher injects the AI Gateway dispatcher for draft generation.
func (s *Service) SetDispatcher(d AIDispatcher) { s.dispatcher = d }

// SetHTTPClient sets the HTTP client used for external web research. When
// nil, http.DefaultClient is used.
func (s *Service) SetHTTPClient(c *http.Client) { s.httpClient = c }

// Enabled reads the housing_guide.enabled system_setting toggle
// (§96 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, pgtype.UUID{}, "housing_guide.enabled", false)
}

// Research runs grounded web research for the given destination, optional
// city, and accommodation type. It returns a formatted "GROUNDED RESEARCH"
// section and a list of unique source URLs to append to the generation
// prompt. Errors are returned so the caller can log and fall back.
func (s *Service) Research(ctx context.Context, tenantID, destSpaceID, citySpaceID pgtype.UUID, accommodationType string) (string, []string, error) {
	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	destName, err := s.store.GetSpaceName(ctx, destSpaceID, tenantID)
	if err != nil {
		return "", nil, err
	}

	dest := destName
	if dest == "" {
		dest = destSpaceID.String()
	}

	city := ""

	if citySpaceID.Valid {
		cityName, err := s.store.GetSpaceName(ctx, citySpaceID, tenantID)
		if err != nil {
			return "", nil, err
		}

		city = cityName
	}

	accom := accommodationType
	if accom == "" {
		accom = "housing"
	} else {
		accom = strings.ReplaceAll(accom, "_", " ")
	}

	rentLoc := dest
	if city != "" {
		rentLoc = city + " " + dest
	}

	queries := []string{
		fmt.Sprintf("current rent ranges for %s in %s 2026", accom, rentLoc),
		fmt.Sprintf("common housing scams in %s 2026", dest),
	}

	return s.runWebResearch(ctx, client, tenantID, queries)
}

// runWebResearch executes the provided queries through the configured
// external search provider and formats the results for the generation
// prompt. It returns the full grounded research section and a deduplicated
// list of citation URLs.
func (s *Service) runWebResearch(ctx context.Context, client *http.Client, tenantID pgtype.UUID, queries []string) (string, []string, error) {
	var sections []string

	seenCitations := make(map[string]bool)

	var citations []string

	for _, q := range queries {
		provider, results, err := ai.SearchWeb(ctx, client, s.settings, tenantID, q, 5)
		if err != nil {
			return "", nil, err
		}

		if len(results) == 0 {
			continue
		}

		section, sectionCitations := buildGroundedResearchSection(q, provider, results)
		sections = append(sections, section)

		for _, c := range sectionCitations {
			if !seenCitations[c] {
				seenCitations[c] = true

				citations = append(citations, c)
			}
		}
	}

	if len(sections) == 0 {
		return "", nil, nil
	}

	var sb strings.Builder

	sb.WriteString("GROUNDED RESEARCH (use the search results below for factual, time-sensitive sub-fields such as rent ranges, scams, and landlord requirements; cite the source URLs when you include factual claims):\n\n")

	for _, sec := range sections {
		sb.WriteString(sec)
		sb.WriteString("\n\n")
	}

	if len(citations) > 0 {
		sb.WriteString("Sources:\n")

		for _, c := range citations {
			sb.WriteString("- " + c + "\n")
		}
	}

	return sb.String(), citations, nil
}

// buildGroundedResearchSection formats a single query's results.
func buildGroundedResearchSection(query, provider string, results []ai.WebSearchResult) (string, []string) {
	if len(results) == 0 {
		return "", nil
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Query: %q (provider: %s)\n", query, provider))

	seen := make(map[string]bool)

	var citations []string

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("\nResult %d:\n", i+1))
		sb.WriteString(fmt.Sprintf("Title: %s\n", r.Title))
		sb.WriteString(fmt.Sprintf("URL: %s\n", r.URL))
		sb.WriteString(fmt.Sprintf("Content: %s\n", r.Content))

		if len(r.Citations) > 0 {
			sb.WriteString(fmt.Sprintf("Citations: %s\n", strings.Join(r.Citations, ", ")))
		}

		for _, c := range append([]string{r.URL}, r.Citations...) {
			if c == "" {
				continue
			}

			if !seen[c] {
				seen[c] = true

				citations = append(citations, c)
			}
		}
	}

	return sb.String(), citations
}

// List returns a paginated, filtered list of published housing guides.
func (s *Service) List(ctx context.Context, tenantID, destSpaceID, citySpaceID pgtype.UUID, accommodationType string, page, limit int32) ([]GuideDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	active := true

	rows, total, err := s.store.ListGuides(ctx, ListGuidesFilter{

		DestSpaceID:       destSpaceID,
		CitySpaceID:       citySpaceID,
		AccommodationType: accommodationType,
		Status:            StatusPublished,
		IsActive:          &active,
		Limit:             limit,
		Offset:            offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("housingguide: list: %w", err)
	}

	dtos := make([]GuideDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}

	return dtos, total, nil
}

// GetByID returns a single published housing guide by id.
func (s *Service) GetByID(ctx context.Context, id, tenantID pgtype.UUID) (GuideDTO, error) {
	row, err := s.store.GetGuideByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GuideDTO{}, pgx.ErrNoRows
		}

		return GuideDTO{}, fmt.Errorf("housingguide: get by id: %w", err)
	}

	return rowToDTO(row), nil
}

// Create creates a new housing guide (admin). Returns the created guide.
func (s *Service) Create(ctx context.Context, tenantID pgtype.UUID, input CreateGuideRequest) (GuideDTO, error) {
	if input.DestinationSpaceID == "" {
		return GuideDTO{}, errors.New("destination_space_id is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return GuideDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var citySpaceID pgtype.UUID
	if input.CitySpaceID != "" {
		if err := citySpaceID.Scan(input.CitySpaceID); err != nil {
			return GuideDTO{}, fmt.Errorf("invalid city_space_id: %w", err)
		}
	}

	searchPlatforms, err := normalizeJSONB(input.SearchPlatforms, []byte("[]"))
	if err != nil {
		return GuideDTO{}, fmt.Errorf("invalid search_platforms: %w", err)
	}

	rentRanges, err := normalizeJSONB(input.RentRanges, []byte("{}"))
	if err != nil {
		return GuideDTO{}, fmt.Errorf("invalid rent_ranges: %w", err)
	}

	storeInput := CreateGuideInput{
		DestinationSpaceID: destSpaceID,
		CitySpaceID:        citySpaceID,
		SearchPlatforms:    searchPlatforms,
		RentRanges:         rentRanges,
		ScamCategories:     input.ScamCategories,
		IsActive:           true,
	}
	if input.AccommodationType != "" {
		storeInput.AccommodationType = &input.AccommodationType
	}

	if input.LandlordRequirements != "" {
		storeInput.LandlordRequirements = &input.LandlordRequirements
	}

	if input.Tips != "" {
		storeInput.Tips = &input.Tips
	}

	if input.Status != "" {
		storeInput.Status = input.Status
	}

	if input.IsActive != nil {
		storeInput.IsActive = *input.IsActive
	}

	if input.AIModel != "" {
		storeInput.AIModel = &input.AIModel
	}

	if input.AIPromptHash != "" {
		storeInput.AIPromptHash = &input.AIPromptHash
	}

	id, err := s.store.CreateGuide(ctx, storeInput)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("housingguide: create: %w", err)
	}

	row, err := s.store.GetGuideByID(ctx, id, tenantID)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("housingguide: create: fetch created: %w", err)
	}

	return rowToDTO(row), nil
}

// Update updates an existing housing guide (admin). Only provided fields
// are applied.
func (s *Service) Update(ctx context.Context, id, tenantID pgtype.UUID, input UpdateGuideRequest) (GuideDTO, error) {
	storeInput := UpdateGuideInput{
		AccommodationType:    input.AccommodationType,
		LandlordRequirements: input.LandlordRequirements,
		Tips:                 input.Tips,
		Status:               input.Status,
		IsActive:             input.IsActive,
		AIModel:              input.AIModel,
		AIPromptHash:         input.AIPromptHash,
		IsJapabotIndexed:     input.IsJapabotIndexed,
		ScamCategories:       input.ScamCategories,
	}

	if input.SearchPlatforms != nil {
		sp, err := normalizeJSONB(input.SearchPlatforms, []byte("[]"))
		if err != nil {
			return GuideDTO{}, fmt.Errorf("invalid search_platforms: %w", err)
		}

		storeInput.SearchPlatforms = sp
	}

	if input.RentRanges != nil {
		rr, err := normalizeJSONB(input.RentRanges, []byte("{}"))
		if err != nil {
			return GuideDTO{}, fmt.Errorf("invalid rent_ranges: %w", err)
		}

		storeInput.RentRanges = rr
	}

	if err := s.store.UpdateGuide(ctx, id, tenantID, storeInput); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GuideDTO{}, pgx.ErrNoRows
		}

		return GuideDTO{}, fmt.Errorf("housingguide: update: %w", err)
	}

	row, err := s.store.GetGuideByID(ctx, id, tenantID)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("housingguide: update: fetch updated: %w", err)
	}

	return rowToDTO(row), nil
}

// Approve performs HITL approval on a housing guide (admin). Sets
// hitl_reviewed_by, hitl_reviewed_at=now(), status='published', and
// optional review notes.
func (s *Service) Approve(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, input ApproveGuideRequest) (GuideDTO, error) {
	if err := s.store.ApproveGuide(ctx, id, tenantID, reviewerID, input.Notes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GuideDTO{}, pgx.ErrNoRows
		}

		return GuideDTO{}, fmt.Errorf("housingguide: approve: %w", err)
	}

	row, err := s.store.GetGuideByID(ctx, id, tenantID)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("housingguide: approve: fetch approved: %w", err)
	}

	// Meilisearch indexing for housing_guides is not yet wired — there is no
	// `housing_guide` entity type in the search indexer and no housing_guides
	// Meilisearch index. When ADR-088 RAG indexing is extended to housing
	// guides, enqueue `search:index_entity` here with payload
	// {"entity_id": id, "entity_type": "housing_guide", "tenant_id": ...}
	// and add a `housing_guide` case to worker.HandleSearchIndexSync.
	// (fire-and-forget, Rule 8 — never block the request on fan-out.)

	return rowToDTO(row), nil
}

// Reject rejects a pending housing guide (admin HITL). Sets status to
// 'rejected' and records the reviewer and review notes.
func (s *Service) Reject(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, reason string) (GuideDTO, error) {
	var notes *string
	if reason != "" {
		notes = &reason
	}

	if err := s.store.RejectGuide(ctx, id, tenantID, reviewerID, notes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GuideDTO{}, pgx.ErrNoRows
		}

		return GuideDTO{}, fmt.Errorf("housingguide: reject: %w", err)
	}

	row, err := s.store.GetGuideByID(ctx, id, tenantID)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("housingguide: reject: fetch rejected: %w", err)
	}

	// No Meilisearch index update — housing_guides is not yet indexed (see
	// comment in Approve). When indexing is added, enqueue a delete here.

	return rowToDTO(row), nil
}

// AdminList returns all housing guides (any status) for the admin
// management queue.
func (s *Service) AdminList(ctx context.Context, tenantID pgtype.UUID, status, accommodationType, q string, destSpaceID, citySpaceID pgtype.UUID, isActive *bool, page, limit int32) ([]GuideDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.AdminList(ctx, AdminListFilter{

		Status:            status,
		AccommodationType: accommodationType,
		DestSpaceID:       destSpaceID,
		CitySpaceID:       citySpaceID,
		Q:                 q,
		IsActive:          isActive,
		Limit:             limit,
		Offset:            offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("housingguide: admin list: %w", err)
	}

	dtos := make([]GuideDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}

	return dtos, total, nil
}

// GenerateDraft creates a housing guide draft via the Content Agent
// (§96, ADR-088). The draft is inserted with status='ai_draft',
// ai_generated=true, ai_model='content_agent_v1', and
// ai_prompt_hash=SHA256(prompt). It requires HITL approval (ApproveGuide)
// before publication. Admin-only (housing_guide.manage permission).
//
// When an AI dispatcher is wired (SetDispatcher), the method calls the AI
// Gateway via the "content_generation" category. If the dispatcher is nil
// or the call fails, the draft is still created with the prompt as tips so
// a reviewer can fill it manually (graceful degradation).
func (s *Service) GenerateDraft(ctx context.Context, tenantID pgtype.UUID, input GenerateGuideRequest) (GenerateDraftResponse, error) {
	if input.DestinationSpaceID == "" {
		return GenerateDraftResponse{}, errors.New("destination_space_id is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return GenerateDraftResponse{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var citySpaceID pgtype.UUID
	if input.CitySpaceID != "" {
		if err := citySpaceID.Scan(input.CitySpaceID); err != nil {
			return GenerateDraftResponse{}, fmt.Errorf("invalid city_space_id: %w", err)
		}
	}

	// Look up the destination (and optional city) names so the prompt and
	// research queries are human-readable. Missing names are non-fatal; we
	// fall back to the supplied IDs.
	destName, _ := s.store.GetSpaceName(ctx, destSpaceID, tenantID)
	cityName := ""

	if citySpaceID.Valid {
		cityName, _ = s.store.GetSpaceName(ctx, citySpaceID, tenantID)
	}

	location := destName
	if location == "" {
		location = input.DestinationSpaceID
	}

	if cityName != "" {
		location = cityName + ", " + location
	}

	// Build the prompt sent to the Content Agent.
	prompt := fmt.Sprintf(
		"Generate a housing guide for destination %s (destination_space_id=%s), accommodation_type=%s. Include tips for finding housing, landlord requirements, common scams, and rent ranges.\n\n"+
			"Use the GROUNDED RESEARCH section below for current, factual sub-fields (rent ranges, scams, landlord requirements, official links). Cite the source URLs when you make factual claims. If no grounded research is available, use your training knowledge and clearly note that figures are estimates.",
		location, input.DestinationSpaceID, input.AccommodationType,
	)

	// Augment the prompt with grounded web research. If research fails, log
	// a warning and continue with the original prompt (graceful degradation).
	if s.researcher != nil {
		researchText, _, researchErr := s.researcher.Research(ctx, tenantID, destSpaceID, citySpaceID, input.AccommodationType)
		if researchErr != nil {
			slog.Warn("housingguide: web research failed, continuing with original prompt", "error", researchErr)
		} else if researchText != "" {
			prompt = prompt + "\n\n" + researchText
		}
	}

	// ai_prompt_hash = SHA256(prompt) for audit traceability.
	sum := sha256.Sum256([]byte(prompt))
	promptHash := hex.EncodeToString(sum[:])

	// Call the AI Gateway when a dispatcher is wired.
	tips := prompt

	if s.dispatcher != nil {
		resp, aiErr := s.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
			Category: "content_generation",
			Payload: map[string]any{
				"prompt":     prompt,
				"tenant_id":  tenantID,
				"guide_type": "housing_guide",
			},
		})
		if aiErr != nil {
			return GenerateDraftResponse{}, fmt.Errorf("housingguide: AI generation failed: %w", aiErr)
		}

		if text, ok := resp.Raw["text"].(string); ok {
			tips = text
		} else if text, ok := resp.Raw["content"].(string); ok {
			tips = text
		} else if text, ok := resp.Raw["output"].(string); ok {
			tips = text
		}

		slog.Info("housingguide: AI Gateway generated draft", "destination_space_id", input.DestinationSpaceID, "accommodation_type", input.AccommodationType, "prompt_hash", promptHash, )
	}

	aiModel := "content_agent_v1"

	var accomTypePtr *string
	if input.AccommodationType != "" {
		accomTypePtr = &input.AccommodationType
	}

	tipsPtr := &tips

	id, err := s.store.CreateGuide(ctx, CreateGuideInput{
		DestinationSpaceID: destSpaceID,
		CitySpaceID:        citySpaceID,
		AccommodationType:  accomTypePtr,
		Tips:               tipsPtr,
		Status:             StatusAIDraft,
		IsActive:           false,
		AIGenerated:        true,
		AIModel:            &aiModel,
		AIPromptHash:       &promptHash,
	})
	if err != nil {
		return GenerateDraftResponse{}, fmt.Errorf("housingguide: generate draft: %w", err)
	}

	return GenerateDraftResponse{
		Status:             "accepted",
		Message:            "Content Agent draft created in 'ai_draft' status. Review and approve to publish.",
		GuideID:            id.String(),
		DestinationSpaceID: destSpaceID.String(),
	}, nil
}

// rowToDTO converts a GuideRow (joined query result) to a GuideDTO.
func rowToDTO(r GuideRow) GuideDTO {
	dto := GuideDTO{
		ID:                 r.ID.String(),
		DestinationSpaceID: r.DestinationSpaceID.String(),
		SearchPlatforms:    normalizeRawMessage(r.SearchPlatforms),
		RentRanges:         normalizeRawMessage(r.RentRanges),
		ScamCategories:     r.ScamCategories,
		Status:             r.Status,
		IsActive:           r.IsActive,
		AIGenerated:        r.AIGenerated,
		IsJapabotIndexed:   r.IsJapabotIndexed,
		CreatedAt:          formatTs(r.CreatedAt),
		UpdatedAt:          formatTs(r.UpdatedAt),
		HITLReviewedAt:     formatTs(r.HITLReviewedAt),
		LastVerifiedAt:     formatDate(r.LastVerifiedAt),
	}
	if r.CitySpaceID.Valid {
		dto.CitySpaceID = r.CitySpaceID.String()
	}

	if r.AccommodationType != nil {
		dto.AccommodationType = *r.AccommodationType
	}

	if r.LandlordRequirements != nil {
		dto.LandlordRequirements = *r.LandlordRequirements
	}

	if r.Tips != nil {
		dto.Tips = *r.Tips
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

	if r.HITLReviewNotes != nil {
		dto.HITLReviewNotes = *r.HITLReviewNotes
	}

	if r.DestSpaceName != nil {
		dto.DestSpaceName = *r.DestSpaceName
	}

	if r.DestSpaceCode != nil {
		dto.DestSpaceCode = *r.DestSpaceCode
	}

	if r.CitySpaceName != nil {
		dto.CitySpaceName = *r.CitySpaceName
	}

	if r.CitySpaceCode != nil {
		dto.CitySpaceCode = *r.CitySpaceCode
	}

	if r.ReviewerName != nil {
		dto.ReviewerName = *r.ReviewerName
	}

	return dto
}

// normalizeJSONB validates/normalizes a json.RawMessage input. If the
// input is empty, the fallback is returned. Invalid JSON returns an
// error.
func normalizeJSONB(in json.RawMessage, fallback []byte) ([]byte, error) {
	if len(in) == 0 {
		return fallback, nil
	}

	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		return nil, err
	}

	return in, nil
}

// normalizeRawMessage returns the raw JSON bytes for the DTO, or nil if
// empty.
func normalizeRawMessage(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}

	return b
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}

	return t.Time.UTC().Format(time.RFC3339)
}

// formatDate renders a pgtype.Date as YYYY-MM-DD, or "" if NULL.
func formatDate(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}

	return d.Time.Format("2006-01-02")
}
