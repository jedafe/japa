// Package visaroutes — service.go orchestrates public listing,
// comparison, detail, and admin CRUD with HITL approval for visa routes
// (§90). Feature-flagged via visa_routes.enabled system_setting.
// // Ref: japaconnect-engineering/sections/90-visa-route-comparison.md
// //	japaconnect-engineering/adr/ADR-098-visa-route-comparison.md
package visaroutes

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

// RouteDTO is the API response shape for a single visa route.
type RouteDTO struct {
	ID                   string `json:"id"`
	TenantID             string `json:"omitempty"`
	DestinationSpaceID   string `json:"destination_space_id"`
	DestinationSpaceName string `json:"destination_space_name,omitempty"`
	DestinationSpaceCode string `json:"destination_space_code,omitempty"`
	VisaCategory         string `json:"visa_category"`
	Name                 string `json:"name"`
	Slug                 string `json:"slug"`
	EligibilitySummary   string `json:"eligibility_summary,omitempty"`
	Requirements         any    `json:"requirements,omitempty"`
	TypicalFeeUSD        any    `json:"typical_fee_usd,omitempty"`
	PathToPR             string `json:"path_to_pr,omitempty"`
	RenewalTerms         string `json:"renewal_terms,omitempty"`
	Restrictions         string `json:"restrictions,omitempty"`
	GovSourceURL         string `json:"gov_source_url"`
	SourceURLLastChecked string `json:"source_url_last_checked,omitempty"`
	Status               string `json:"status"`
	IsActive             bool   `json:"is_active"`
	AIGenerated          bool   `json:"ai_generated"`
	AIModel              string `json:"ai_model,omitempty"`
	AIPromptHash         string `json:"ai_prompt_hash,omitempty"`
	HITLReviewedBy       string `json:"hitl_reviewed_by,omitempty"`
	HITLReviewedAt       string `json:"hitl_reviewed_at,omitempty"`
	HITLReviewNotes      string `json:"hitl_review_notes,omitempty"`
	IsJapabotIndexed     bool   `json:"is_japabot_indexed"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

// CreateRouteInput is the request body for POST /admin/visa-routes.
type CreateRouteInput struct {
	DestinationSpaceID   string `json:"destination_space_id"`
	VisaCategory         string `json:"visa_category"`
	Name                 string `json:"name"`
	Slug                 string `json:"slug"`
	EligibilitySummary   string `json:"eligibility_summary"`
	Requirements         any    `json:"requirements"`
	TypicalFeeUSD        any    `json:"typical_fee_usd"`
	PathToPR             string `json:"path_to_pr"`
	RenewalTerms         string `json:"renewal_terms"`
	Restrictions         string `json:"restrictions"`
	GovSourceURL         string `json:"gov_source_url"`
	SourceURLLastChecked string `json:"source_url_last_checked"`
	Status               string `json:"status"`
	IsActive             *bool  `json:"is_active"`
	AIGenerated          *bool  `json:"ai_generated"`
	AIModel              string `json:"ai_model"`
	AIPromptHash         string `json:"ai_prompt_hash"`
}

// UpdateRouteInput is the request body for PATCH /admin/visa-routes/:id.
// All fields optional; only provided fields are updated.
type UpdateRouteInput struct {
	DestinationSpaceID   *string `json:"destination_space_id"`
	VisaCategory         *string `json:"visa_category"`
	Name                 *string `json:"name"`
	Slug                 *string `json:"slug"`
	EligibilitySummary   *string `json:"eligibility_summary"`
	Requirements         any     `json:"requirements"`
	TypicalFeeUSD        any     `json:"typical_fee_usd"`
	PathToPR             *string `json:"path_to_pr"`
	RenewalTerms         *string `json:"renewal_terms"`
	Restrictions         *string `json:"restrictions"`
	GovSourceURL         *string `json:"gov_source_url"`
	SourceURLLastChecked *string `json:"source_url_last_checked"`
	Status               *string `json:"status"`
	IsActive             *bool   `json:"is_active"`
	AIGenerated          *bool   `json:"ai_generated"`
	AIModel              *string `json:"ai_model"`
	AIPromptHash         *string `json:"ai_prompt_hash"`
}

// ApproveInput is the request body for PATCH /admin/visa-routes/:id/approve.
type ApproveInput struct {
	Notes *string `json:"notes"`
}

// GenerateDraftResponse is the response from Content Agent draft generation.
type GenerateDraftResponse struct {
	Status             string `json:"status"`
	Message            string `json:"message"`
	RouteID            string `json:"route_id"`
	DestinationSpaceID string `json:"destination_space_id"`
	VisaCategory       string `json:"visa_category"`
}

// GenerateRouteRequest is the request body for POST
// /admin/visa-routes/generate (Content Agent draft placeholder, §90,
// ADR-088).
type GenerateRouteRequest struct {
	DestinationSpaceID string `json:"destination_space_id"`
	VisaCategory       string `json:"visa_category"`
	Prompt             string `json:"prompt"`
}

// AIDispatcher is the subset of the AI Gateway the visa routes service
// needs to generate draft routes (content_generation category).
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}

// Service holds the business logic for visa routes.
type Service struct {
	store      *Store
	settings   *config.Settings
	dispatcher AIDispatcher
}

// NewService creates a visa routes service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetDispatcher injects the AI Gateway dispatcher for draft generation.
func (s *Service) SetDispatcher(d AIDispatcher) { s.dispatcher = d }

// Enabled reads the visa_routes.enabled system_setting toggle (§90
// feature flag).
func (s *Service) Enabled(ctx context.Context) bool {
	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "visa_routes.enabled", false)
}

// List returns a paginated, filtered list of published+active visa routes.
func (s *Service) List(ctx context.Context, destSpaceID pgtype.UUID, profession, education string, experienceYears, page, limit int32) ([]RouteDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.List(ctx, ListFilter{

		DestinationSpaceID: destSpaceID,
		Profession:         profession,
		Education:          education,
		ExperienceYears:    experienceYears,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("visaroutes: list: %w", err)
	}

	dtos := make([]RouteDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}

	return dtos, total, nil
}

// Compare returns up to 4 visa routes by ID for side-by-side comparison.
func (s *Service) Compare(ctx context.Context, ids []pgtype.UUID) ([]RouteDTO, error) {
	if len(ids) == 0 {
		return []RouteDTO{}, nil
	}

	if len(ids) > 4 {
		ids = ids[:4]
	}

	rows, err := s.store.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("visaroutes: compare: %w", err)
	}

	dtos := make([]RouteDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}

	return dtos, nil
}

// GetByID returns a single published+active visa route for public view.
func (s *Service) GetByID(ctx context.Context, id pgtype.UUID) (RouteDTO, error) {
	row, err := s.store.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RouteDTO{}, pgx.ErrNoRows
		}

		return RouteDTO{}, fmt.Errorf("visaroutes: get by id: %w", err)
	}

	if row.Status != StatusPublished || !row.IsActive {
		return RouteDTO{}, pgx.ErrNoRows
	}

	return rowToDTO(row), nil
}

// GetBySlug returns a single published, active visa route by its slug.
func (s *Service) GetBySlug(ctx context.Context, slug string) (RouteDTO, error) {
	row, err := s.store.GetBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RouteDTO{}, pgx.ErrNoRows
		}

		return RouteDTO{}, fmt.Errorf("visaroutes: get by slug: %w", err)
	}

	if row.Status != StatusPublished || !row.IsActive {
		return RouteDTO{}, pgx.ErrNoRows
	}

	return rowToDTO(row), nil
}

// AdminList returns all visa routes (any status) for the admin view.
func (s *Service) AdminList(ctx context.Context, destSpaceID pgtype.UUID, status, visaCategory, q, sort, order string, isActive *bool, page, limit int32) ([]RouteDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.AdminList(ctx, AdminListFilter{

		DestinationSpaceID: destSpaceID,
		Status:             status,
		IsActive:           isActive,
		VisaCategory:       visaCategory,
		Q:                  q,
		Sort:               sort,
		Order:              order,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("visaroutes: admin list: %w", err)
	}

	dtos := make([]RouteDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}

	return dtos, total, nil
}

// Create creates a new visa route (admin).
func (s *Service) Create(ctx context.Context, input CreateRouteInput) (RouteDTO, error) {
	if input.Name == "" {
		return RouteDTO{}, errors.New("name is required")
	}

	if input.VisaCategory == "" {
		return RouteDTO{}, errors.New("visa_category is required")
	}

	if input.GovSourceURL == "" {
		return RouteDTO{}, errors.New("gov_source_url is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return RouteDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	requirementsJSON, err := marshalRequirements(input.Requirements)
	if err != nil {
		return RouteDTO{}, fmt.Errorf("invalid requirements: %w", err)
	}

	var typicalFee *float64

	if input.TypicalFeeUSD != nil {
		f, ok := toFloat64(input.TypicalFeeUSD)
		if ok {
			typicalFee = &f
		}
	}

	var eligibilitySummary *string
	if input.EligibilitySummary != "" {
		eligibilitySummary = &input.EligibilitySummary
	}

	var pathToPR *string
	if input.PathToPR != "" {
		pathToPR = &input.PathToPR
	}

	var renewalTerms *string
	if input.RenewalTerms != "" {
		renewalTerms = &input.RenewalTerms
	}

	var restrictions *string
	if input.Restrictions != "" {
		restrictions = &input.Restrictions
	}

	var aiModel *string
	if input.AIModel != "" {
		aiModel = &input.AIModel
	}

	var aiPromptHash *string
	if input.AIPromptHash != "" {
		aiPromptHash = &input.AIPromptHash
	}

	var sourceLastChecked pgtype.Date
	if input.SourceURLLastChecked != "" {
		if err := sourceLastChecked.Scan(input.SourceURLLastChecked); err != nil {
			return RouteDTO{}, fmt.Errorf("invalid source_url_last_checked: %w", err)
		}
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	aiGenerated := false
	if input.AIGenerated != nil {
		aiGenerated = *input.AIGenerated
	}

	row, err := s.store.Create(ctx, CreateRoute{
		DestinationSpaceID:   destSpaceID,
		VisaCategory:         input.VisaCategory,
		Name:                 input.Name,
		Slug:                 input.Slug,
		EligibilitySummary:   eligibilitySummary,
		RequirementsJSON:     requirementsJSON,
		TypicalFeeUSD:        typicalFee,
		PathToPR:             pathToPR,
		RenewalTerms:         renewalTerms,
		Restrictions:         restrictions,
		GovSourceURL:         input.GovSourceURL,
		SourceURLLastChecked: sourceLastChecked,
		Status:               input.Status,
		IsActive:             isActive,
		AIGenerated:          aiGenerated,
		AIModel:              aiModel,
		AIPromptHash:         aiPromptHash,
	})
	if err != nil {
		return RouteDTO{}, fmt.Errorf("visaroutes: create: %w", err)
	}

	return rowToDTO(row), nil
}

// Update applies a partial update to a visa route (admin).
func (s *Service) Update(ctx context.Context, id pgtype.UUID, input UpdateRouteInput) (RouteDTO, error) {
	update := UpdateRoute{}

	if input.DestinationSpaceID != nil && *input.DestinationSpaceID != "" {
		var dest pgtype.UUID
		if err := dest.Scan(*input.DestinationSpaceID); err != nil {
			return RouteDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
		}

		update.DestinationSpaceID = &dest
	}

	if input.VisaCategory != nil {
		update.VisaCategory = input.VisaCategory
	}

	if input.Name != nil {
		update.Name = input.Name
	}

	if input.Slug != nil {
		update.Slug = input.Slug
	}

	if input.EligibilitySummary != nil {
		update.EligibilitySummary = input.EligibilitySummary
	}

	if input.Requirements != nil {
		rj, err := marshalRequirements(input.Requirements)
		if err != nil {
			return RouteDTO{}, fmt.Errorf("invalid requirements: %w", err)
		}

		update.RequirementsJSON = rj
	}

	if input.TypicalFeeUSD != nil {
		f, ok := toFloat64(input.TypicalFeeUSD)
		if ok {
			update.TypicalFeeUSD = &f
		}
	}

	if input.PathToPR != nil {
		update.PathToPR = input.PathToPR
	}

	if input.RenewalTerms != nil {
		update.RenewalTerms = input.RenewalTerms
	}

	if input.Restrictions != nil {
		update.Restrictions = input.Restrictions
	}

	if input.GovSourceURL != nil {
		update.GovSourceURL = input.GovSourceURL
	}

	if input.SourceURLLastChecked != nil && *input.SourceURLLastChecked != "" {
		var d pgtype.Date
		if err := d.Scan(*input.SourceURLLastChecked); err != nil {
			return RouteDTO{}, fmt.Errorf("invalid source_url_last_checked: %w", err)
		}

		update.SourceURLLastChecked = &d
	}

	if input.Status != nil {
		update.Status = input.Status
	}

	if input.IsActive != nil {
		update.IsActive = input.IsActive
	}

	if input.AIGenerated != nil {
		update.AIGenerated = input.AIGenerated
	}

	if input.AIModel != nil {
		update.AIModel = input.AIModel
	}

	if input.AIPromptHash != nil {
		update.AIPromptHash = input.AIPromptHash
	}

	row, err := s.store.Update(ctx, id, update)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RouteDTO{}, pgx.ErrNoRows
		}

		return RouteDTO{}, fmt.Errorf("visaroutes: update: %w", err)
	}

	return rowToDTO(row), nil
}

// Approve marks a visa route as HITL-reviewed and published (admin).
// Returns the updated route DTO per the ADR-088 HITL approve pattern.
func (s *Service) Approve(ctx context.Context, id, reviewerID pgtype.UUID, input ApproveInput) (RouteDTO, error) {
	row, err := s.store.Approve(ctx, id, reviewerID, input.Notes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RouteDTO{}, pgx.ErrNoRows
		}

		return RouteDTO{}, fmt.Errorf("visaroutes: approve: %w", err)
	}

	return rowToDTO(row), nil
}

// Deactivate soft-deletes a visa route by setting is_active = false (admin).
func (s *Service) Deactivate(ctx context.Context, id pgtype.UUID) error {
	if err := s.store.Deactivate(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("visaroutes: deactivate: %w", err)
	}

	return nil
}

// GenerateDraft creates a visa route draft via the Content Agent (§90,
// ADR-088). The draft is inserted with status='ai_draft',
// ai_generated=true, ai_model='content_agent_v1', and
// ai_prompt_hash=SHA256(prompt). It requires HITL approval before
// publication. Admin-only (visa_routes.manage permission).
// // When an AI dispatcher is wired (SetDispatcher), the method calls the AI
// Gateway via the "content_generation" category. If the dispatcher is nil,
// the draft is still created with placeholder content so a reviewer can
// fill it manually (graceful degradation).
func (s *Service) GenerateDraft(ctx context.Context, input GenerateRouteRequest) (GenerateDraftResponse, error) {
	if input.DestinationSpaceID == "" {
		return GenerateDraftResponse{}, errors.New("destination_space_id is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return GenerateDraftResponse{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	visaCategory := input.VisaCategory
	if visaCategory == "" {
		visaCategory = "general"
	}

	// Build the prompt sent to the Content Agent.
	prompt := input.Prompt
	if prompt == "" {
		prompt = fmt.Sprintf(
			"Generate a visa route guide for destination_space_id=%s, visa_category=%s. Include eligibility summary, requirements, typical fees, path to permanent residency, renewal terms, restrictions, and government source URL.",
			input.DestinationSpaceID, visaCategory,
		)
	}

	// ai_prompt_hash = SHA256(prompt) for audit traceability.
	sum := sha256.Sum256([]byte(prompt))
	promptHash := hex.EncodeToString(sum[:])

	// Call the AI Gateway when a dispatcher is wired.
	eligibilitySummary := prompt

	if s.dispatcher != nil {
		resp, aiErr := s.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
			Category: "content_generation",
			Payload: map[string]any{
				"prompt": prompt,

				"guide_type": "visa_route",
			},
		})
		if aiErr != nil {
			return GenerateDraftResponse{}, fmt.Errorf("visaroutes: AI generation failed: %w", aiErr)
		}

		if text, ok := resp.Raw["text"].(string); ok {
			eligibilitySummary = text
		} else if text, ok := resp.Raw["content"].(string); ok {
			eligibilitySummary = text
		} else if text, ok := resp.Raw["output"].(string); ok {
			eligibilitySummary = text
		}

		slog.Info("visaroutes: AI Gateway generated draft", "destination_space_id", input.DestinationSpaceID, "visa_category", visaCategory, "prompt_hash", promptHash)
	}

	aiModel := "content_agent_v1"
	aiGenerated := true
	routeName := fmt.Sprintf("AI Draft: %s (%s)", visaCategory, input.DestinationSpaceID)
	slug := fmt.Sprintf("ai-draft-%s-%s", visaCategory, promptHash[:8])
	isActive := false

	row, err := s.store.Create(ctx, CreateRoute{
		DestinationSpaceID: destSpaceID,
		VisaCategory:       visaCategory,
		Name:               routeName,
		Slug:               slug,
		EligibilitySummary: &eligibilitySummary,
		Status:             StatusAIDraft,
		IsActive:           isActive,
		AIGenerated:        aiGenerated,
		AIModel:            &aiModel,
		AIPromptHash:       &promptHash,
	})
	if err != nil {
		return GenerateDraftResponse{}, fmt.Errorf("visaroutes: generate draft: %w", err)
	}

	return GenerateDraftResponse{
		Status:             "accepted",
		Message:            "Content Agent draft created in 'ai_draft' status. Review and approve to publish.",
		RouteID:            row.ID.String(),
		DestinationSpaceID: destSpaceID.String(),
		VisaCategory:       visaCategory,
	}, nil
}

// rowToDTO converts a RouteRow to a RouteDTO.
func rowToDTO(r RouteRow) RouteDTO {
	dto := RouteDTO{
		ID:                 r.ID.String(),
		DestinationSpaceID: r.DestinationSpaceID.String(),
		VisaCategory:       r.VisaCategory,
		Name:               r.Name,
		Slug:               r.Slug,
		GovSourceURL:       r.GovSourceURL,
		Status:             r.Status,
		IsActive:           r.IsActive,
		AIGenerated:        r.AIGenerated,
		IsJapabotIndexed:   r.IsJapabotIndexed,
		CreatedAt:          formatTs(r.CreatedAt),
		UpdatedAt:          formatTs(r.UpdatedAt),
		Requirements:       parseRequirements(r.RequirementsJSON),
	}
	if r.DestSpaceName != nil {
		dto.DestinationSpaceName = *r.DestSpaceName
	}

	if r.DestSpaceCode != nil {
		dto.DestinationSpaceCode = *r.DestSpaceCode
	}

	if r.EligibilitySummary != nil {
		dto.EligibilitySummary = *r.EligibilitySummary
	}

	if r.TypicalFeeUSD != nil {
		dto.TypicalFeeUSD = *r.TypicalFeeUSD
	}

	if r.PathToPR != nil {
		dto.PathToPR = *r.PathToPR
	}

	if r.RenewalTerms != nil {
		dto.RenewalTerms = *r.RenewalTerms
	}

	if r.Restrictions != nil {
		dto.Restrictions = *r.Restrictions
	}

	if r.SourceURLLastChecked.Valid {
		dto.SourceURLLastChecked = r.SourceURLLastChecked.Time.Format("2006-01-02")
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

// marshalRequirements converts the input (any) into a JSON []byte for
// storage. nil/empty values produce an empty JSON object.
func marshalRequirements(v any) ([]byte, error) {
	if v == nil {
		return []byte("{}"), nil
	}

	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return b, nil
}

// parseRequirements decodes the JSON []byte into a generic value for the
// DTO. Invalid/empty payloads yield nil.
func parseRequirements(b []byte) any {
	if len(b) == 0 {
		return nil
	}

	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}

	return v
}

// toFloat64 coerces a numeric any (float64, int, json.Number) to float64.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false
		}

		return f, true
	}

	return 0, false
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}

	return t.Time.UTC().Format(time.RFC3339)
}
