// Package attestationguide — service.go orchestrates attestation
// pathway lookup, creation, HITL approval, and community update
// moderation. Pathways are curated guides for document
// legalization/apostille across jurisdictions. Feature-flagged via the
// attestation_guide.enabled system_setting (§97, ADR-105).
//
// Ref: japaconnect-engineering/sections/97-attestation-guide.md
//
//	japaconnect-engineering/adr/ADR-105-attestation-guide.md
package attestationguide

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/config"
)

// AIDispatcher is the subset of the AI Gateway the attestation guide
// needs to generate draft pathways. *ai.Dispatcher satisfies this.
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}

// PathwayDTO is the API response shape for a single attestation pathway.
type PathwayDTO struct {
	ID                    string          `json:"id"`
	DocumentType          string          `json:"document_type"`
	IssuingSpaceID        string          `json:"issuing_space_id"`
	IssuingSpaceName      string          `json:"issuing_space_name,omitempty"`
	IssuingSpaceCode      string          `json:"issuing_space_code,omitempty"`
	DestinationSpaceID    string          `json:"destination_space_id"`
	DestinationSpaceName  string          `json:"destination_space_name,omitempty"`
	DestinationSpaceCode  string          `json:"destination_space_code,omitempty"`
	ApostilleRequired     bool            `json:"apostille_required"`
	PathwaySteps          json.RawMessage `json:"pathway_steps"`
	CommunityLastVerified string          `json:"community_last_verified,omitempty"`
	Status                string          `json:"status"`
	IsActive              bool            `json:"is_active"`
	AIGenerated           bool            `json:"ai_generated"`
	AIModel               string          `json:"ai_model,omitempty"`
	AIPromptHash          string          `json:"ai_prompt_hash,omitempty"`
	HITLReviewedBy        string          `json:"hitl_reviewed_by,omitempty"`
	HITLReviewedAt        string          `json:"hitl_reviewed_at,omitempty"`
	HITLReviewNotes       string          `json:"hitl_review_notes,omitempty"`
	IsJapabotIndexed      bool            `json:"is_japabot_indexed"`
	CreatedAt             string          `json:"created_at"`
	UpdatedAt             string          `json:"updated_at"`
}

// CommunityUpdateDTO is the API response shape for a community update.
type CommunityUpdateDTO struct {
	ID             string `json:"id"`
	PathwayID      string `json:"pathway_id"`
	UserID         string `json:"user_id"`
	StepIndex      *int32 `json:"step_index,omitempty"`
	FeeEstimate    string `json:"fee_estimate,omitempty"`
	TurnaroundDays *int32 `json:"turnaround_days,omitempty"`
	Note           string `json:"note,omitempty"`
	Status         string `json:"status"`
	ReviewedBy     string `json:"reviewed_by,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// CreatePathwayInput is the request body for POST /admin/attestation.
type CreatePathwayInput struct {
	DocumentType          string          `json:"document_type"`
	IssuingSpaceID        string          `json:"issuing_space_id"`
	DestinationSpaceID    string          `json:"destination_space_id"`
	ApostilleRequired     bool            `json:"apostille_required"`
	PathwaySteps          json.RawMessage `json:"pathway_steps"`
	CommunityLastVerified string          `json:"community_last_verified"`
	Status                string          `json:"status"`
	IsActive              *bool           `json:"is_active"`
	AIModel               string          `json:"ai_model"`
	AIPromptHash          string          `json:"ai_prompt_hash"`
	HITLReviewNotes       string          `json:"hitl_review_notes"`
}

// CreateCommunityUpdateInput is the request body for
// POST /attestation/:id/updates.
type CreateCommunityUpdateInput struct {
	StepIndex      *int32 `json:"step_index"`
	FeeEstimate    string `json:"fee_estimate"`
	TurnaroundDays *int32 `json:"turnaround_days"`
	Note           string `json:"note"`
}

// GeneratePathwayInput is the request body for the Content Agent draft
// endpoint (POST /admin/attestation/generate). Per ADR-088, the body
// includes a prompt field and the handler stores ai_prompt_hash = SHA256(prompt).
type GeneratePathwayInput struct {
	DocumentType       string `json:"document_type"`
	IssuingSpaceID     string `json:"issuing_space_id"`
	DestinationSpaceID string `json:"destination_space_id"`
	Prompt             string `json:"prompt"`
}

// Service holds the business logic for the Attestation Guide.
type Service struct {
	store      *Store
	settings   *config.Settings
	dispatcher AIDispatcher
}

// NewService creates an attestation guide service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// SetDispatcher injects the AI Gateway dispatcher for draft generation
// (called at server wiring time).
func (s *Service) SetDispatcher(d AIDispatcher) { s.dispatcher = d }

// Enabled reads the attestation_guide.enabled system_setting toggle
// (§97 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "attestation_guide.enabled", false)
}

// isHagueMember reports whether the destination space is a Hague Convention
// member per the attestation_guide.hague_members system_setting (§97 Hague
// Convention section). The setting is a JSON array of space IDs. Returns false
// on any error (fail-safe: no apostille required if lookup fails).
func (s *Service) isHagueMember(ctx context.Context, tenantID, destSpaceID pgtype.UUID) bool {
	if s.settings == nil || !destSpaceID.Valid {
		return false
	}
	raw := s.settings.GetStringOr(ctx, tenantID, "attestation_guide.hague_members", "[]")
	var members []string
	if err := json.Unmarshal([]byte(raw), &members); err != nil {
		return false
	}
	destID := destSpaceID.String()
	for _, m := range members {
		if m == destID {
			return true
		}
	}
	return false
}

// List returns a paginated, filtered list of published attestation
// pathways. Anonymous-accessible.
func (s *Service) List(ctx context.Context, tenantID, destSpaceID, issuingSpaceID pgtype.UUID, documentType string, page, limit int32) ([]PathwayDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.ListPathways(ctx, ListPathwaysFilter{
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		DocumentType:       documentType,
		IssuingSpaceID:     issuingSpaceID,
		Status:             PathwayStatusPublished,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("attestationguide: list: %w", err)
	}

	dtos := make([]PathwayDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, pathwayRowToDTO(r))
	}
	return dtos, total, nil
}

// GetByID returns a single published attestation pathway by id.
// Anonymous-accessible. Only returns pathways with status='published' and
// is_active=true (BUG-001: prevent leaking unpublished pathways).
func (s *Service) GetByID(ctx context.Context, id, tenantID pgtype.UUID) (PathwayDTO, error) {
	row, err := s.store.GetPublishedPathwayByID(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return PathwayDTO{}, pgx.ErrNoRows
		}
		return PathwayDTO{}, fmt.Errorf("attestationguide: get by id: %w", err)
	}
	return pathwayRowToDTO(row), nil
}

// Create creates a new attestation pathway. Admin-only
// (attestation_guide.manage permission checked at route level).
func (s *Service) Create(ctx context.Context, tenantID pgtype.UUID, input CreatePathwayInput) (PathwayDTO, error) {
	if input.DocumentType == "" {
		return PathwayDTO{}, fmt.Errorf("document_type is required")
	}
	if input.IssuingSpaceID == "" {
		return PathwayDTO{}, fmt.Errorf("issuing_space_id is required")
	}
	if input.DestinationSpaceID == "" {
		return PathwayDTO{}, fmt.Errorf("destination_space_id is required")
	}

	var issuingSpaceID pgtype.UUID
	if err := issuingSpaceID.Scan(input.IssuingSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("invalid issuing_space_id: %w", err)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	steps := []byte(input.PathwaySteps)
	if len(steps) == 0 {
		steps = []byte("[]")
	}

	// §97 Hague Convention: if apostille_required was not explicitly provided,
	// derive it from the attestation_guide.hague_members system_setting based
	// on the destination country. Fix BUG-002.
	apostilleRequired := input.ApostilleRequired
	if !apostilleRequired {
		apostilleRequired = s.isHagueMember(ctx, tenantID, destSpaceID)
	}

	p := Pathway{
		TenantID:           tenantID,
		DocumentType:       input.DocumentType,
		IssuingSpaceID:     issuingSpaceID,
		DestinationSpaceID: destSpaceID,
		ApostilleRequired:  apostilleRequired,
		PathwaySteps:       steps,
		Status:             input.Status,
		IsActive:           true,
	}
	if input.IsActive != nil {
		p.IsActive = *input.IsActive
	}
	if input.AIModel != "" {
		p.AIModel = &input.AIModel
	}
	if input.AIPromptHash != "" {
		p.AIPromptHash = &input.AIPromptHash
	}
	if input.HITLReviewNotes != "" {
		p.HITLReviewNotes = &input.HITLReviewNotes
	}
	if input.CommunityLastVerified != "" {
		if err := p.CommunityLastVerified.Scan(input.CommunityLastVerified); err != nil {
			return PathwayDTO{}, fmt.Errorf("invalid community_last_verified: %w", err)
		}
	}

	created, err := s.store.CreatePathway(ctx, p)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("attestationguide: create: %w", err)
	}

	row, err := s.store.GetPathwayByID(ctx, created.ID, tenantID)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("attestationguide: create: reload: %w", err)
	}
	return pathwayRowToDTO(row), nil
}

// GenerateDraft creates an AI-drafted pathway in 'ai_draft' status. The
// pathway is marked ai_generated=true, stores ai_prompt_hash = SHA256(prompt)
// per ADR-088, and requires HITL approval (Approve) before publication. The
// pathway_steps are populated via the AI Gateway (§20 content_generation
// category); if the dispatcher is not wired or the call fails, the draft is
// created with empty steps so the reviewer can fill them manually. Admin-only
// (attestation_guide.manage permission).
func (s *Service) GenerateDraft(ctx context.Context, tenantID pgtype.UUID, input GeneratePathwayInput) (PathwayDTO, error) {
	if input.DocumentType == "" || input.IssuingSpaceID == "" || input.DestinationSpaceID == "" {
		return PathwayDTO{}, fmt.Errorf("document_type, issuing_space_id, and destination_space_id are required")
	}

	var issuingSpaceID pgtype.UUID
	if err := issuingSpaceID.Scan(input.IssuingSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("invalid issuing_space_id: %w", err)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	// Build the generation prompt.
	prompt := input.Prompt
	if prompt == "" {
		prompt = fmt.Sprintf("Generate an attestation pathway for document type '%s' from %s to %s",
			input.DocumentType, input.IssuingSpaceID, input.DestinationSpaceID)
	}

	aiModel := "content_agent_v1"
	// BUG-007: compute ai_prompt_hash = SHA256(prompt) per ADR-088 for audit
	// reproducibility.
	promptHash := ""
	if prompt != "" {
		h := sha256.Sum256([]byte(prompt))
		promptHash = hex.EncodeToString(h[:])
	}

	// Call the AI Gateway to populate pathway_steps. The dispatcher is
	// optional (nil during tests); if it is nil, the draft is created with
	// empty steps. If the dispatcher is wired and the call fails, the
	// generation fails hard — the admin should retry rather than receive
	// an empty draft they must manually fill.
	pathwaySteps := []byte("[]")
	if s.dispatcher != nil {
		resp, derr := s.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
			Category: "content_generation",
			Payload: map[string]any{
				"prompt":    prompt,
				"tenant_id": tenantID,
			},
		})
		if derr != nil {
			return PathwayDTO{}, fmt.Errorf("attestationguide: AI generation failed: %w", derr)
		}
		pathwaySteps = parsePathwayStepsFromAIResponse(resp)
	}

	p := Pathway{
		TenantID:           tenantID,
		DocumentType:       input.DocumentType,
		IssuingSpaceID:     issuingSpaceID,
		DestinationSpaceID: destSpaceID,
		ApostilleRequired:  false, // determined by Hague Convention lookup at approve time
		PathwaySteps:       pathwaySteps,
		Status:             PathwayStatusAIDraft,
		IsActive:           false, // not active until approved
		AIGenerated:        true,
		AIModel:            &aiModel,
	}
	if promptHash != "" {
		p.AIPromptHash = &promptHash
	}

	created, err := s.store.CreatePathway(ctx, p)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("attestationguide: generate draft: %w", err)
	}

	row, err := s.store.GetPathwayByID(ctx, created.ID, tenantID)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("attestationguide: generate draft: reload: %w", err)
	}
	return pathwayRowToDTO(row), nil
}

// parsePathwayStepsFromAIResponse extracts pathway steps from the AI
// gateway response. The response may contain structured JSON or plain
// text; we wrap text into a single-step array for the reviewer to refine.
func parsePathwayStepsFromAIResponse(resp ai.Response) []byte {
	if text, ok := resp.Raw["text"].(string); ok && text != "" {
		wrapped, _ := json.Marshal([]map[string]string{{"step": text}})
		return wrapped
	}
	if text, ok := resp.Raw["content"].(string); ok && text != "" {
		wrapped, _ := json.Marshal([]map[string]string{{"step": text}})
		return wrapped
	}
	if output, ok := resp.Raw["output"].(string); ok && output != "" {
		wrapped, _ := json.Marshal([]map[string]string{{"step": output}})
		return wrapped
	}
	return []byte("[]")
}

// Approve performs the HITL approval on an AI-drafted pathway. Sets
// hitl_reviewed_by, hitl_reviewed_at = now(), status = 'published'.
// Also recomputes apostille_required from the Hague Convention lookup
// (§97). Admin-only (attestation_guide.manage permission).
func (s *Service) Approve(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, notes string) (PathwayDTO, error) {
	// Load the pathway to determine the destination country for the Hague
	// Convention lookup (BUG-002/BUG-003).
	row, err := s.store.GetPathwayByID(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return PathwayDTO{}, pgx.ErrNoRows
		}
		return PathwayDTO{}, fmt.Errorf("attestationguide: approve: lookup: %w", err)
	}

	apostilleRequired := s.isHagueMember(ctx, tenantID, row.DestinationSpaceID)

	var notesPtr *string
	if notes != "" {
		notesPtr = &notes
	}
	if err := s.store.ApprovePathway(ctx, id, tenantID, reviewerID, apostilleRequired, notesPtr); err != nil {
		if err == pgx.ErrNoRows {
			return PathwayDTO{}, pgx.ErrNoRows
		}
		return PathwayDTO{}, fmt.Errorf("attestationguide: approve: %w", err)
	}

	row, err = s.store.GetPathwayByID(ctx, id, tenantID)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("attestationguide: approve: reload: %w", err)
	}
	return pathwayRowToDTO(row), nil
}

// SubmitCommunityUpdate creates a pending community update for a
// pathway. Auth required (attestation_guide.update permission). Only
// published, active pathways accept community updates (BUG-002).
func (s *Service) SubmitCommunityUpdate(ctx context.Context, pathwayID, tenantID, userID pgtype.UUID, input CreateCommunityUpdateInput) (CommunityUpdateDTO, error) {
	// Verify the pathway exists, belongs to the tenant, and is published +
	// active (BUG-002: no updates on draft/ai_draft/pending/rejected pathways).
	if _, err := s.store.GetPublishedPathwayByID(ctx, pathwayID, tenantID); err != nil {
		if err == pgx.ErrNoRows {
			return CommunityUpdateDTO{}, pgx.ErrNoRows
		}
		return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: submit update: lookup pathway: %w", err)
	}

	u := CommunityUpdate{
		PathwayID:      pathwayID,
		UserID:         userID,
		StepIndex:      input.StepIndex,
		TurnaroundDays: input.TurnaroundDays,
		Status:         UpdateStatusPending,
	}
	if input.FeeEstimate != "" {
		u.FeeEstimate = &input.FeeEstimate
	}
	if input.Note != "" {
		u.Note = &input.Note
	}

	created, err := s.store.CreateCommunityUpdate(ctx, u)
	if err != nil {
		return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: submit update: %w", err)
	}
	return communityUpdateToDTO(created), nil
}

// ReviewCommunityUpdate approves or rejects a pending community update.
// On approval, the update's fee_estimate/turnaround_days are applied to the
// parent pathway's pathway_steps at the step_index, and community_last_verified
// is bumped (BUG-003 + BUG-004). Admin-only (attestation_guide.manage
// permission).
func (s *Service) ReviewCommunityUpdate(ctx context.Context, updateID, tenantID, reviewerID pgtype.UUID, approve bool) (CommunityUpdateDTO, error) {
	status := UpdateStatusApproved
	if !approve {
		status = UpdateStatusRejected
	}
	if err := s.store.ReviewCommunityUpdate(ctx, updateID, tenantID, reviewerID, status); err != nil {
		if err == pgx.ErrNoRows {
			return CommunityUpdateDTO{}, pgx.ErrNoRows
		}
		return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: review update: %w", err)
	}
	// Reload the full row so the DTO includes all fields (BUG-005 fix).
	row, err := s.store.GetCommunityUpdateByID(ctx, updateID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return CommunityUpdateDTO{}, pgx.ErrNoRows
		}
		return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: review update: reload: %w", err)
	}

	// BUG-003 + BUG-004: on approval, apply the update to the parent pathway
	// (patch pathway_steps + bump community_last_verified).
	if approve && row.PathwayID.Valid {
		if err := s.store.ApplyApprovedUpdateToPathway(ctx, row.PathwayID, tenantID, row.StepIndex, row.FeeEstimate, row.TurnaroundDays); err != nil {
			return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: review update: apply: %w", err)
		}
	}

	return communityUpdateToDTO(row), nil
}

// AdminList returns all attestation pathways (any status) for the
// moderation queue. Admin-only (attestation_guide.manage permission).
// Supports search (q) and sort (sortBy/sortDir) per BUG-004.
func (s *Service) AdminList(ctx context.Context, tenantID, destSpaceID pgtype.UUID, status, documentType string, isActive *bool, search, sortBy, sortDir string, page, limit int32) ([]PathwayDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.AdminList(ctx, AdminListFilter{
		TenantID:           tenantID,
		Status:             status,
		DocumentType:       documentType,
		DestinationSpaceID: destSpaceID,
		IsActive:           isActive,
		Search:             search,
		SortBy:             sortBy,
		SortDir:            sortDir,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("attestationguide: admin list: %w", err)
	}

	dtos := make([]PathwayDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, pathwayRowToDTO(r))
	}
	return dtos, total, nil
}

// AdminListUpdates returns pending (or filtered) community updates for the
// admin moderation queue (BUG-005). Admin-only (attestation_guide.manage).
func (s *Service) AdminListUpdates(ctx context.Context, tenantID pgtype.UUID, status string, page, limit int32) ([]CommunityUpdateDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit
	if status == "" {
		status = UpdateStatusPending
	}

	rows, total, err := s.store.ListCommunityUpdates(ctx, ListCommunityUpdatesFilter{
		TenantID: tenantID,
		Status:   status,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("attestationguide: admin list updates: %w", err)
	}

	dtos := make([]CommunityUpdateDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, communityUpdateToDTO(r))
	}
	return dtos, total, nil
}

// pathwayRowToDTO converts a PathwayRow (joined query result) to a
// PathwayDTO.
func pathwayRowToDTO(r PathwayRow) PathwayDTO {
	dto := PathwayDTO{
		ID:                r.ID.String(),
		DocumentType:      r.DocumentType,
		ApostilleRequired: r.ApostilleRequired,
		PathwaySteps:      json.RawMessage(r.PathwaySteps),
		Status:            r.Status,
		IsActive:          r.IsActive,
		AIGenerated:       r.AIGenerated,
		IsJapabotIndexed:  r.IsJapabotIndexed,
		CreatedAt:         formatTs(r.CreatedAt),
		UpdatedAt:         formatTs(r.UpdatedAt),
	}
	if r.IssuingSpaceID.Valid {
		dto.IssuingSpaceID = r.IssuingSpaceID.String()
	}
	if r.IssuingSpaceName != nil {
		dto.IssuingSpaceName = *r.IssuingSpaceName
	}
	if r.IssuingSpaceCode != nil {
		dto.IssuingSpaceCode = *r.IssuingSpaceCode
	}
	if r.DestinationSpaceID.Valid {
		dto.DestinationSpaceID = r.DestinationSpaceID.String()
	}
	if r.DestSpaceName != nil {
		dto.DestinationSpaceName = *r.DestSpaceName
	}
	if r.DestSpaceCode != nil {
		dto.DestinationSpaceCode = *r.DestSpaceCode
	}
	if r.CommunityLastVerified.Valid {
		dto.CommunityLastVerified = r.CommunityLastVerified.Time.Format("2006-01-02")
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

// communityUpdateToDTO converts a CommunityUpdate row to a DTO.
func communityUpdateToDTO(u CommunityUpdate) CommunityUpdateDTO {
	dto := CommunityUpdateDTO{
		Status:    u.Status,
		CreatedAt: formatTs(u.CreatedAt),
	}
	if u.ID.Valid {
		dto.ID = u.ID.String()
	}
	if u.PathwayID.Valid {
		dto.PathwayID = u.PathwayID.String()
	}
	if u.UserID.Valid {
		dto.UserID = u.UserID.String()
	}
	dto.StepIndex = u.StepIndex
	if u.FeeEstimate != nil {
		dto.FeeEstimate = *u.FeeEstimate
	}
	dto.TurnaroundDays = u.TurnaroundDays
	if u.Note != nil {
		dto.Note = *u.Note
	}
	if u.ReviewedBy.Valid {
		dto.ReviewedBy = u.ReviewedBy.String()
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
