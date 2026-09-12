// Package credentialchecker — service.go orchestrates credential pathway
// lookup, community note submission, and admin HITL review workflow for
// the Professional Credential Recognition Checker (§94, ADR-102). It
// manages credential_pathways and credential_community_notes via the
// local Store. Feature-flagged via credential_checker.enabled
// system_setting (§94).
//
// Ref: japaconnect-engineering/sections/94-professional-credential-checker.md
//
//	japaconnect-engineering/adr/ADR-102-credential-checker.md
package credentialchecker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// ProfessionDTO is the API response shape for a single profession.
type ProfessionDTO struct {
	ID       string `json:"id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Category string `json:"category,omitempty"`
}

// PathwayDTO is the API response shape for a single credential pathway.
type PathwayDTO struct {
	ID                     string      `json:"id"`
	ProfessionID           pgtype.UUID `json:"profession_id"`
	ProfessionSlug         string      `json:"profession_slug"`
	ProfessionName         string      `json:"profession_name"`
	ProfessionCategory     string      `json:"profession_category,omitempty"`
	IssuingSpaceID         string      `json:"issuing_space_id"`
	IssuingSpaceName       string      `json:"issuing_space_name,omitempty"`
	IssuingSpaceCode       string      `json:"issuing_space_code,omitempty"`
	DestinationSpaceID     string      `json:"destination_space_id"`
	DestinationSpaceName   string      `json:"destination_space_name,omitempty"`
	DestinationSpaceCode   string      `json:"destination_space_code,omitempty"`
	RegulatoryBody         string      `json:"regulatory_body"`
	RegulatoryBodyURL      string      `json:"regulatory_body_url,omitempty"`
	PathwayType            string      `json:"pathway_type"`
	TimelineEstimate       string      `json:"timeline_estimate,omitempty"`
	CostEstimateUSD        string      `json:"cost_estimate_usd,omitempty"`
	PathwayDescription     string      `json:"pathway_description"`
	OfficialApplicationURL string      `json:"official_application_url,omitempty"`
	SourceURLLastChecked   string      `json:"source_url_last_checked,omitempty"`
	Status                 string      `json:"status"`
	IsActive               bool        `json:"is_active"`
	AIGenerated            bool        `json:"ai_generated"`
	AIModel                string      `json:"ai_model,omitempty"`
	AIPromptHash           string      `json:"ai_prompt_hash,omitempty"`
	HITLReviewedBy         string      `json:"hitl_reviewed_by,omitempty"`
	HITLReviewedAt         string      `json:"hitl_reviewed_at,omitempty"`
	HITLReviewNotes        string      `json:"hitl_review_notes,omitempty"`
	IsJapabotIndexed       bool        `json:"is_japabot_indexed"`
	CreatedAt              string      `json:"created_at"`
	UpdatedAt              string      `json:"updated_at"`
}

// CommunityNoteDTO is the API response shape for a single community note.
type CommunityNoteDTO struct {
	ID        string `json:"id"`
	PathwayID string `json:"pathway_id"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name,omitempty"`
	Note      string `json:"note"`
	Upvotes   int32  `json:"upvotes"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// PathwayDetailDTO is the response for GET /credentials/:id — a pathway
// plus its published community notes.
type PathwayDetailDTO struct {
	Pathway PathwayDTO         `json:"pathway"`
	Notes   []CommunityNoteDTO `json:"notes"`
}

// CreatePathwayRequest is the request body for POST /admin/credentials.
type CreatePathwayRequest struct {
	ProfessionID           pgtype.UUID `json:"profession_id"`
	IssuingSpaceID         string      `json:"issuing_space_id"`
	DestinationSpaceID     string      `json:"destination_space_id"`
	RegulatoryBody         string      `json:"regulatory_body"`
	RegulatoryBodyURL      string      `json:"regulatory_body_url"`
	PathwayType            string      `json:"pathway_type"`
	TimelineEstimate       string      `json:"timeline_estimate"`
	CostEstimateUSD        string      `json:"cost_estimate_usd"`
	PathwayDescription     string      `json:"pathway_description"`
	OfficialApplicationURL string      `json:"official_application_url"`
	SourceURLLastChecked   string      `json:"source_url_last_checked"`
	Status                 string      `json:"status"`
	IsActive               *bool       `json:"is_active"`
	AIGenerated            *bool       `json:"ai_generated"`
	AIModel                string      `json:"ai_model"`
	AIPromptHash           string      `json:"ai_prompt_hash"`
}

// UpdatePathwayRequest is the request body for PATCH /admin/credentials/:id.
// All fields are optional (pointer); only provided fields are updated.
// is_japabot_indexed is intentionally absent — it is a GENERATED ALWAYS AS
// column computed from status + is_active and cannot be updated directly.
type UpdatePathwayRequest struct {
	ProfessionID           *pgtype.UUID `json:"profession_id"`
	IssuingSpaceID         *string      `json:"issuing_space_id"`
	DestinationSpaceID     *string      `json:"destination_space_id"`
	RegulatoryBody         *string      `json:"regulatory_body"`
	RegulatoryBodyURL      *string      `json:"regulatory_body_url"`
	PathwayType            *string      `json:"pathway_type"`
	TimelineEstimate       *string      `json:"timeline_estimate"`
	CostEstimateUSD        *string      `json:"cost_estimate_usd"`
	PathwayDescription     *string      `json:"pathway_description"`
	OfficialApplicationURL *string      `json:"official_application_url"`
	SourceURLLastChecked   *string      `json:"source_url_last_checked"`
	Status                 *string      `json:"status"`
	IsActive               *bool        `json:"is_active"`
}

// CreateNoteRequest is the request body for POST /credentials/:id/notes.
type CreateNoteRequest struct {
	Note string `json:"note"`
}

// ApprovePathwayRequest is the optional body for PATCH
// /admin/credentials/:id/approve.
type ApprovePathwayRequest struct {
	ReviewNotes *string `json:"review_notes"`
}

// GeneratePathwayRequest is the request body for POST
// /admin/credentials/generate — the Content Agent draft placeholder.
type GeneratePathwayRequest struct {
	ProfessionID       pgtype.UUID `json:"profession_id"`
	IssuingSpaceID     string      `json:"issuing_space_id"`
	DestinationSpaceID string      `json:"destination_space_id"`
}

// GeneratePathwayResponse is the response for the Content Agent draft
// generation endpoint. Returns the created pathway ID so the admin can
// navigate to the draft for HITL review.
type GeneratePathwayResponse struct {
	Status             string      `json:"status"`
	Message            string      `json:"message"`
	PathwayID          string      `json:"pathway_id,omitempty"`
	ProfessionID       pgtype.UUID `json:"profession_id,omitempty"`
	IssuingSpaceID     string      `json:"issuing_space_id,omitempty"`
	DestinationSpaceID string      `json:"destination_space_id,omitempty"`
}

// Service holds the business logic for the Credential Checker.
type Service struct {
	store        *Store
	settings     *config.Settings
	aiDispatcher AIDispatcher
}

// NewService creates a credential checker service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// SetAIDispatcher injects the AI Gateway dispatcher used by
// GeneratePathwayDraft to call the Content Agent (§20, ADR-088).
// Called after NewService once the dispatcher is available.
func (s *Service) SetAIDispatcher(d AIDispatcher) { s.aiDispatcher = d }

// AIDispatcher is the subset of *ai.Dispatcher the credential checker
// service calls to generate pathway drafts. Local interface to avoid
// importing internal/ai (which would create an import cycle).
// Ref: §20, ADR-088 — same pattern as cvformatter.AIDispatcher.
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req AIRequest) (AIResponse, error)
	HasCategory(category string) bool
}

// AIRequest is the provider-agnostic request envelope passed to the AI
// dispatcher. The adapter in internal/server converts this to ai.Request.
type AIRequest struct {
	SystemPrompt string
	UserPrompt   string
}

// AIResponse is the provider-agnostic response envelope returned by the
// AI dispatcher.
type AIResponse struct {
	Text  string
	Model string
}

// ListProfessions returns all credential professions as DTOs.
func (s *Service) ListProfessions(ctx context.Context) ([]ProfessionDTO, error) {
	rows, err := s.store.ListProfessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("credentialchecker: list professions: %w", err)
	}

	dtos := make([]ProfessionDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, professionRowToDTO(r))
	}

	return dtos, nil
}

// GetProfessionBySlug returns a single profession by slug. Used by the
// Studio routing adapter to resolve a profession from the job's
// guide_category or topic_prompt.
func (s *Service) GetProfessionBySlug(ctx context.Context, slug string) (ProfessionRow, error) {
	return s.store.GetProfessionBySlug(ctx, slug)
}

// CreateProfession creates a new credential profession. credential_professions
// is a global table and ignores the supplied tenant_id.
func (s *Service) CreateProfession(ctx context.Context, tenantID pgtype.UUID, input CreateProfessionInput) (ProfessionDTO, error) {
	if input.Slug == "" {
		return ProfessionDTO{}, errors.New("slug is required")
	}

	if input.Name == "" {
		return ProfessionDTO{}, errors.New("name is required")
	}

	row, err := s.store.CreateProfession(ctx, input)
	if err != nil {
		return ProfessionDTO{}, fmt.Errorf("credentialchecker: create profession: %w", err)
	}

	return professionRowToDTO(row), nil
}

// UpdateProfession updates a credential profession by id.
func (s *Service) UpdateProfession(ctx context.Context, id, tenantID pgtype.UUID, input UpdateProfessionInput) (ProfessionDTO, error) {
	if input.Slug == nil && input.Name == nil && input.Category == nil {
		return ProfessionDTO{}, errors.New("no fields to update")
	}

	row, err := s.store.UpdateProfession(ctx, id, input)
	if err != nil {
		return ProfessionDTO{}, fmt.Errorf("credentialchecker: update profession: %w", err)
	}

	return professionRowToDTO(row), nil
}

// DeleteProfession deletes a credential profession by id.
func (s *Service) DeleteProfession(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.DeleteProfession(ctx, id); err != nil {
		return fmt.Errorf("credentialchecker: delete profession: %w", err)
	}

	return nil
}

// ListPathways returns a paginated, filtered list of published credential
// pathways. The destination/profession/issuing filters are optional.
func (s *Service) ListPathways(ctx context.Context, tenantID, destSpaceID, issuingSpaceID, professionID pgtype.UUID, page, limit int32) ([]PathwayDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.ListPathways(ctx, ListPathwaysFilter{

		DestinationSpaceID: destSpaceID,
		ProfessionID:       professionID,
		IssuingSpaceID:     issuingSpaceID,
		Status:             StatusPublished,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("credentialchecker: list pathways: %w", err)
	}

	dtos := make([]PathwayDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, pathwayRowToDTO(r))
	}

	return dtos, total, nil
}

// GetPathway returns a single pathway by id plus its published community
// notes. Only published pathways are returned to public callers — draft,
// ai_draft, pending, and rejected pathways are hidden (ADR-088: ai_draft
// status is NEVER visible to end users).
func (s *Service) GetPathway(ctx context.Context, id, tenantID pgtype.UUID) (PathwayDetailDTO, error) {
	row, err := s.store.GetPathwayByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PathwayDetailDTO{}, pgx.ErrNoRows
		}

		return PathwayDetailDTO{}, fmt.Errorf("credentialchecker: get pathway: %w", err)
	}

	if row.Status != StatusPublished {
		return PathwayDetailDTO{}, pgx.ErrNoRows
	}

	notes, err := s.store.GetPathwayNotes(ctx, id, tenantID)
	if err != nil {
		slog.Error("credentialchecker: get pathway notes failed", "pathway_id", id, "error", err)

		notes = nil
	}

	noteDTOs := make([]CommunityNoteDTO, 0, len(notes))
	for _, n := range notes {
		noteDTOs = append(noteDTOs, noteRowToDTO(n))
	}

	return PathwayDetailDTO{
		Pathway: pathwayRowToDTO(row),
		Notes:   noteDTOs,
	}, nil
}

// CreatePathway creates a new credential pathway. Admin-only
// (credential_checker.manage permission checked at route level).
func (s *Service) CreatePathway(ctx context.Context, tenantID pgtype.UUID, req CreatePathwayRequest) (PathwayDTO, error) {
	if req.RegulatoryBody == "" {
		return PathwayDTO{}, errors.New("regulatory_body is required")
	}

	if req.PathwayDescription == "" {
		return PathwayDTO{}, errors.New("pathway_description is required")
	}

	if req.PathwayType == "" {
		return PathwayDTO{}, errors.New("pathway_type is required")
	}

	var issuingSpaceID pgtype.UUID
	if err := issuingSpaceID.Scan(req.IssuingSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("invalid issuing_space_id: %w", err)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(req.DestinationSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	input := CreatePathwayInput{

		ProfessionID:       req.ProfessionID,
		IssuingSpaceID:     issuingSpaceID,
		DestinationSpaceID: destSpaceID,
		RegulatoryBody:     req.RegulatoryBody,
		PathwayType:        req.PathwayType,
		PathwayDescription: req.PathwayDescription,
		Status:             req.Status,
		IsActive:           true,
	}
	if req.IsActive != nil {
		input.IsActive = *req.IsActive
	}

	if req.AIGenerated != nil {
		input.AIGenerated = *req.AIGenerated
	}

	if req.RegulatoryBodyURL != "" {
		input.RegulatoryBodyURL = &req.RegulatoryBodyURL
	}

	if req.TimelineEstimate != "" {
		input.TimelineEstimate = &req.TimelineEstimate
	}

	if req.CostEstimateUSD != "" {
		input.CostEstimateUSD = &req.CostEstimateUSD
	}

	if req.OfficialApplicationURL != "" {
		input.OfficialApplicationURL = &req.OfficialApplicationURL
	}

	if req.SourceURLLastChecked != "" {
		var d pgtype.Date
		if err := d.Scan(req.SourceURLLastChecked); err == nil {
			input.SourceURLLastChecked = d
		}
	}

	if req.AIModel != "" {
		input.AIModel = &req.AIModel
	}

	if req.AIPromptHash != "" {
		input.AIPromptHash = &req.AIPromptHash
	}

	id, err := s.store.CreatePathway(ctx, input)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("credentialchecker: create pathway: %w", err)
	}

	row, err := s.store.GetPathwayByID(ctx, id, tenantID)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("credentialchecker: fetch created pathway: %w", err)
	}

	return pathwayRowToDTO(row), nil
}

// UpdatePathway updates the mutable fields on a credential pathway.
// Admin-only (credential_checker.manage permission checked at route
// level). Only provided (non-nil) fields are updated.
func (s *Service) UpdatePathway(ctx context.Context, id, tenantID pgtype.UUID, req UpdatePathwayRequest) (PathwayDTO, error) {
	input := UpdatePathwayInput{
		ProfessionID:           req.ProfessionID,
		RegulatoryBody:         req.RegulatoryBody,
		RegulatoryBodyURL:      req.RegulatoryBodyURL,
		PathwayType:            req.PathwayType,
		TimelineEstimate:       req.TimelineEstimate,
		CostEstimateUSD:        req.CostEstimateUSD,
		PathwayDescription:     req.PathwayDescription,
		OfficialApplicationURL: req.OfficialApplicationURL,
		Status:                 req.Status,
		IsActive:               req.IsActive,
	}

	if req.IssuingSpaceID != nil && *req.IssuingSpaceID != "" {
		var sid pgtype.UUID
		if err := sid.Scan(*req.IssuingSpaceID); err != nil {
			return PathwayDTO{}, fmt.Errorf("invalid issuing_space_id: %w", err)
		}

		input.IssuingSpaceID = &sid
	}

	if req.DestinationSpaceID != nil && *req.DestinationSpaceID != "" {
		var sid pgtype.UUID
		if err := sid.Scan(*req.DestinationSpaceID); err != nil {
			return PathwayDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
		}

		input.DestinationSpaceID = &sid
	}

	if req.SourceURLLastChecked != nil && *req.SourceURLLastChecked != "" {
		var d pgtype.Date
		if err := d.Scan(*req.SourceURLLastChecked); err == nil {
			input.SourceURLLastChecked = &d
		}
	}

	if err := s.store.UpdatePathway(ctx, id, tenantID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PathwayDTO{}, pgx.ErrNoRows
		}

		return PathwayDTO{}, fmt.Errorf("credentialchecker: update pathway: %w", err)
	}

	row, err := s.store.GetPathwayByID(ctx, id, tenantID)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("credentialchecker: fetch updated pathway: %w", err)
	}

	return pathwayRowToDTO(row), nil
}

// DeletePathway deletes a credential pathway by id for the tenant.
func (s *Service) DeletePathway(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.DeletePathway(ctx, id, tenantID); err != nil {
		return fmt.Errorf("credentialchecker: delete pathway: %w", err)
	}

	return nil
}

// ApprovePathway marks a pathway as HITL-approved by the given reviewer.
// Admin-only (credential_checker.manage permission checked at route
// level). Sets hitl_reviewed_by, hitl_reviewed_at = now(), status =
// 'published'.
func (s *Service) ApprovePathway(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, notes *string) (PathwayDTO, error) {
	if err := s.store.ApprovePathway(ctx, id, tenantID, reviewerID, notes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PathwayDTO{}, pgx.ErrNoRows
		}

		return PathwayDTO{}, fmt.Errorf("credentialchecker: approve pathway: %w", err)
	}

	row, err := s.store.GetPathwayByID(ctx, id, tenantID)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("credentialchecker: fetch approved pathway: %w", err)
	}

	return pathwayRowToDTO(row), nil
}

// CreateNote submits a community note on a pathway. Auth required
// (credential_checker.note permission checked at route level). Notes may
// only be submitted on published pathways — draft/ai_draft/pending
// pathways are not publicly visible and should not accumulate notes.
func (s *Service) CreateNote(ctx context.Context, pathwayID, tenantID, userID pgtype.UUID, note string) (CommunityNoteDTO, error) {
	if note == "" {
		return CommunityNoteDTO{}, errors.New("note is required")
	}

	pathway, err := s.store.GetPathwayByID(ctx, pathwayID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CommunityNoteDTO{}, pgx.ErrNoRows
		}

		return CommunityNoteDTO{}, fmt.Errorf("credentialchecker: note: get pathway: %w", err)
	}

	if pathway.Status != StatusPublished {
		return CommunityNoteDTO{}, pgx.ErrNoRows
	}

	id, createdAt, err := s.store.CreateNote(ctx, pathwayID, userID, note)
	if err != nil {
		return CommunityNoteDTO{}, fmt.Errorf("credentialchecker: create note: %w", err)
	}

	return CommunityNoteDTO{
		ID:        id.String(),
		PathwayID: pathwayID.String(),
		UserID:    userID.String(),
		Note:      note,
		Upvotes:   0,
		Status:    StatusPublished,
		CreatedAt: formatTs(createdAt),
	}, nil
}

// GeneratePathwayDraft creates an AI-drafted credential pathway in
// 'ai_draft' status via the AI Gateway Content Agent (§20, ADR-088).
// The draft is marked ai_generated=true with ai_model and ai_prompt_hash
// set from the generation call. It requires HITL approval (ApprovePathway)
// before publication. Admin-only (credential_checker.manage permission
// checked at route level). Ref: §20, ADR-088, geo-aware-tool-pattern.md.
//
// If no AI provider is registered for the "content_generation" category,
// the method gracefully falls back to placeholder content so the draft
// can still be created and reviewed manually (§21 stub-degradation).
func (s *Service) GeneratePathwayDraft(ctx context.Context, tenantID pgtype.UUID, req GeneratePathwayRequest) (GeneratePathwayResponse, error) {
	var issuingSpaceID pgtype.UUID
	if err := issuingSpaceID.Scan(req.IssuingSpaceID); err != nil {
		return GeneratePathwayResponse{}, fmt.Errorf("invalid issuing_space_id: %w", err)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(req.DestinationSpaceID); err != nil {
		return GeneratePathwayResponse{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	// Fetch profession and space names to build a rich prompt.
	prof, err := s.store.GetProfessionByID(ctx, req.ProfessionID)
	if err != nil {
		return GeneratePathwayResponse{}, fmt.Errorf("credentialchecker: generate draft: get profession: %w", err)
	}

	issuingName, destName, err := s.store.GetSpaceNames(ctx, issuingSpaceID, destSpaceID)
	if err != nil {
		return GeneratePathwayResponse{}, fmt.Errorf("credentialchecker: generate draft: get space names: %w", err)
	}

	// Build the prompt for the Content Agent (§20).
	systemPrompt := `You are a Content Agent for a professional credential recognition checker.
Generate a concise, factual pathway description for recognizing a professional credential
in the destination country. Include the regulatory body name, recognition pathway type,
timeline estimate, cost estimate, and a step-by-step description.
Respond with ONLY a JSON object (no markdown fences) in this format:
{"regulatory_body": "...", "pathway_type": "...", "pathway_description": "...", "timeline_estimate": "...", "cost_estimate_usd": "..."}`

	userPrompt := fmt.Sprintf(
		"Profession: %s\nIssuing country: %s\nDestination country: %s\n\nGenerate the credential recognition pathway.",
		prof.Name, issuingName, destName,
	)

	// Compute ai_prompt_hash = SHA256(prompt) for audit reproducibility (ADR-088).
	promptForHash := systemPrompt + "\n" + userPrompt
	promptHash := computePromptHash(promptForHash)

	// Defaults — used when no AI provider is registered (graceful fallback).
	aiModel := "content_agent_v1"
	regulatoryBody := "AI-generated — pending review"
	pathwayType := PathwayDirect
	pathwayDescription := "AI-generated pathway description — pending HITL review."

	// Call the AI Gateway if a content_generation provider is registered.
	if s.aiDispatcher != nil && s.aiDispatcher.HasCategory("content_generation") {
		resp, err := s.aiDispatcher.Dispatch(ctx, "content_generation", AIRequest{
			SystemPrompt: systemPrompt,
			UserPrompt:   userPrompt,
		})
		if err != nil {
			slog.Warn("credentialchecker: AI dispatch failed, using placeholder",
				"error", err, "profession_id", req.ProfessionID)
		} else if resp.Text != "" {
			// Try to parse the JSON response; fall back to raw text as description.
			if parsed, ok := parseAIPathwayResponse(resp.Text); ok {
				if parsed.RegulatoryBody != "" {
					regulatoryBody = parsed.RegulatoryBody
				}

				if parsed.PathwayType != "" {
					pathwayType = parsed.PathwayType
				}

				if parsed.PathwayDescription != "" {
					pathwayDescription = parsed.PathwayDescription
				}
			} else {
				pathwayDescription = resp.Text
			}

			if resp.Model != "" {
				aiModel = resp.Model
			}
		}
	} else {
		slog.Info("credentialchecker: no content_generation provider, using placeholder draft",
			"profession_id", req.ProfessionID)
	}

	input := CreatePathwayInput{

		ProfessionID:       req.ProfessionID,
		IssuingSpaceID:     issuingSpaceID,
		DestinationSpaceID: destSpaceID,
		RegulatoryBody:     regulatoryBody,
		PathwayType:        pathwayType,
		PathwayDescription: pathwayDescription,
		Status:             StatusAIDraft,
		IsActive:           false,
		AIGenerated:        true,
		AIModel:            &aiModel,
		AIPromptHash:       &promptHash,
	}

	id, err := s.store.CreatePathway(ctx, input)
	if err != nil {
		return GeneratePathwayResponse{}, fmt.Errorf("credentialchecker: generate draft: %w", err)
	}

	slog.Info(
		"credentialchecker: content agent draft created",
		"tenant_id", tenantID,
		"pathway_id", id.String(),
		"profession_id", req.ProfessionID,
		"ai_model", aiModel,
	)

	return GeneratePathwayResponse{
		Status:             "accepted",
		Message:            "Content Agent draft created in 'ai_draft' status. Review and approve to publish.",
		PathwayID:          id.String(),
		ProfessionID:       req.ProfessionID,
		IssuingSpaceID:     req.IssuingSpaceID,
		DestinationSpaceID: req.DestinationSpaceID,
	}, nil
}

// computePromptHash returns the hex-encoded SHA-256 of the prompt string
// for audit reproducibility (ADR-088).
func computePromptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

// aiPathwayResponse is the JSON shape the Content Agent is asked to return.
type aiPathwayResponse struct {
	RegulatoryBody     string `json:"regulatory_body"`
	PathwayType        string `json:"pathway_type"`
	PathwayDescription string `json:"pathway_description"`
	TimelineEstimate   string `json:"timeline_estimate"`
	CostEstimateUSD    string `json:"cost_estimate_usd"`
}

// parseAIPathwayResponse attempts to parse the AI response as JSON.
// Returns false if the response is not valid JSON (caller falls back
// to using the raw text as the pathway description).
func parseAIPathwayResponse(text string) (aiPathwayResponse, bool) {
	text = cleanJSONResponse(text)

	var r aiPathwayResponse

	if err := json.Unmarshal([]byte(text), &r); err != nil {
		return aiPathwayResponse{}, false
	}

	return r, true
}

// cleanJSONResponse strips markdown code fences from an AI response
// so it can be parsed as JSON.
func cleanJSONResponse(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
	}

	if strings.HasSuffix(text, "```") {
		text = text[:len(text)-3]
	}

	return strings.TrimSpace(text)
}

// AdminListPathways returns all pathways (any status) for the admin
// management view. Admin-only (credential_checker.manage permission
// checked at route level).
func (s *Service) AdminListPathways(ctx context.Context, tenantID, destSpaceID, issuingSpaceID, professionID pgtype.UUID, status string, page, limit int32) ([]PathwayDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.ListPathways(ctx, ListPathwaysFilter{

		DestinationSpaceID: destSpaceID,
		ProfessionID:       professionID,
		IssuingSpaceID:     issuingSpaceID,
		Status:             status,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("credentialchecker: admin list pathways: %w", err)
	}

	dtos := make([]PathwayDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, pathwayRowToDTO(r))
	}

	return dtos, total, nil
}

// ---------------------------------------------------------------------------
// DTO conversion helpers
// ---------------------------------------------------------------------------

// professionRowToDTO converts a ProfessionRow to a ProfessionDTO.
func professionRowToDTO(r ProfessionRow) ProfessionDTO {
	dto := ProfessionDTO{
		ID:   r.ID.String(),
		Slug: r.Slug,
		Name: r.Name,
	}
	if r.Category != nil {
		dto.Category = *r.Category
	}

	return dto
}

// pathwayRowToDTO converts a PathwayRow to a PathwayDTO.
func pathwayRowToDTO(r PathwayRow) PathwayDTO {
	dto := PathwayDTO{
		ID:                 r.ID.String(),
		ProfessionID:       r.ProfessionID,
		ProfessionSlug:     r.ProfessionSlug,
		ProfessionName:     r.ProfessionName,
		IssuingSpaceID:     r.IssuingSpaceID.String(),
		DestinationSpaceID: r.DestinationSpaceID.String(),
		RegulatoryBody:     r.RegulatoryBody,
		PathwayType:        r.PathwayType,
		PathwayDescription: r.PathwayDescription,
		Status:             r.Status,
		IsActive:           r.IsActive,
		AIGenerated:        r.AIGenerated,
		IsJapabotIndexed:   r.IsJapabotIndexed,
		CreatedAt:          formatTs(r.CreatedAt),
		UpdatedAt:          formatTs(r.UpdatedAt),
		HITLReviewedAt:     formatTs(r.HITLReviewedAt),
	}
	if r.ProfessionCategory != nil {
		dto.ProfessionCategory = *r.ProfessionCategory
	}

	if r.IssuingSpaceName != nil {
		dto.IssuingSpaceName = *r.IssuingSpaceName
	}

	if r.IssuingSpaceCode != nil {
		dto.IssuingSpaceCode = *r.IssuingSpaceCode
	}

	if r.DestinationSpaceName != nil {
		dto.DestinationSpaceName = *r.DestinationSpaceName
	}

	if r.DestinationSpaceCode != nil {
		dto.DestinationSpaceCode = *r.DestinationSpaceCode
	}

	if r.RegulatoryBodyURL != nil {
		dto.RegulatoryBodyURL = *r.RegulatoryBodyURL
	}

	if r.TimelineEstimate != nil {
		dto.TimelineEstimate = *r.TimelineEstimate
	}

	if r.CostEstimateUSD != nil {
		dto.CostEstimateUSD = *r.CostEstimateUSD
	}

	if r.OfficialApplicationURL != nil {
		dto.OfficialApplicationURL = *r.OfficialApplicationURL
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

	if r.HITLReviewNotes != nil {
		dto.HITLReviewNotes = *r.HITLReviewNotes
	}

	return dto
}

// noteRowToDTO converts a CommunityNoteRow to a CommunityNoteDTO.
func noteRowToDTO(r CommunityNoteRow) CommunityNoteDTO {
	dto := CommunityNoteDTO{
		ID:        r.ID.String(),
		PathwayID: r.PathwayID.String(),
		UserID:    r.UserID.String(),
		Note:      r.Note,
		Upvotes:   r.Upvotes,
		Status:    r.Status,
		CreatedAt: formatTs(r.CreatedAt),
	}
	if r.UserName != nil {
		dto.UserName = *r.UserName
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

// parseProfessionID parses a profession_id query parameter string into
// a pgtype.UUID. Returns an invalid pgtype.UUID (no filter) if the
// string is empty or invalid.
func parseProfessionID(v string) pgtype.UUID {
	if v == "" {
		return pgtype.UUID{}
	}

	var id pgtype.UUID
	if err := id.Scan(v); err != nil {
		return pgtype.UUID{}
	}

	return id
}
