// Package attestationguide — service.go orchestrates attestation
// pathway lookup, creation, HITL approval, and community update
// moderation. Pathways are curated guides for document
// legalization/apostille across jurisdictions. Feature-flagged via the
// attestation_guide.enabled system_setting (§97, ADR-105).
// // Ref: japaconnect-engineering/sections/97-attestation-guide.md
// //	japaconnect-engineering/adr/ADR-105-attestation-guide.md
package attestationguide

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

// AIDispatcher is the subset of the AI Gateway the attestation guide
// needs to generate draft pathways. *ai.Dispatcher satisfies this.
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}

// Researcher runs grounded web research for an issuing jurisdiction,
// destination jurisdiction, and document type. *Service implements this
// interface, but it can be swapped in tests.
type Researcher interface {
	Research(ctx context.Context, destSpaceID, issuingSpaceID pgtype.UUID, documentType string) (string, []string, error)
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
	httpClient *http.Client // nil = http.DefaultClient
	researcher Researcher   // nil = use s.Research
	checker    FeatureChecker
}

// NewService creates an attestation guide service.
func NewService(store *Store, settings *config.Settings) *Service {
	s := &Service{
		store:    store,
		settings: settings,
	}
	s.researcher = s

	return s
}

// SetDispatcher injects the AI Gateway dispatcher for draft generation
// (called at server wiring time).
func (s *Service) SetDispatcher(d AIDispatcher) { s.dispatcher = d }

// SetHTTPClient sets the HTTP client used for external web research. When
// nil, http.DefaultClient is used.
func (s *Service) SetHTTPClient(c *http.Client) { s.httpClient = c }

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for reachability (Ref: internal/features/features.go).
// system_settings is consulted only as a fallback when no checker is injected.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// SetFeatureChecker injects the feature flag checker so Enabled reads the
// feature_flags table instead of drifting against system_settings.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// Enabled reads the attestation_guide.enabled flag (§97). Prefers the
// injected FeatureChecker; falls back to system_settings for backward
// compatibility.
func (s *Service) Enabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "attestation_guide.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "attestation_guide.enabled", false)
}

// Research runs grounded web research for the given issuing jurisdiction,
// destination jurisdiction, and document type. It returns a formatted
// "GROUNDED RESEARCH" section and a list of source URLs to append to the
// generation prompt. Errors are returned so the caller can log and fall back.
func (s *Service) Research(ctx context.Context, destSpaceID, issuingSpaceID pgtype.UUID, documentType string) (string, []string, error) {
	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	destName, err := s.store.GetSpaceName(ctx, destSpaceID)
	if err != nil {
		return "", nil, err
	}

	dest := destName
	if dest == "" {
		dest = destSpaceID.String()
	}

	issuingName, err := s.store.GetSpaceName(ctx, issuingSpaceID)
	if err != nil {
		return "", nil, err
	}

	issuing := issuingName
	if issuing == "" {
		issuing = issuingSpaceID.String()
	}

	doc := documentType
	if doc == "" {
		doc = "document"
	} else {
		doc = strings.ReplaceAll(doc, "_", " ")
	}

	queries := []string{
		fmt.Sprintf("current attestation requirements for %s from %s to %s 2026", doc, issuing, dest),
	}

	return s.runWebResearch(ctx, client, queries)
}

// runWebResearch executes the provided queries through the configured
// external search provider and formats the results for the generation
// prompt. It returns the full grounded research section and a deduplicated
// list of citation URLs.
func (s *Service) runWebResearch(ctx context.Context, client *http.Client, queries []string) (string, []string, error) {
	var sections []string

	seenCitations := make(map[string]bool)

	var citations []string

	for _, q := range queries {
		provider, results, err := ai.SearchWeb(ctx, client, s.settings, q, 5)
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

	sb.WriteString("GROUNDED RESEARCH (use the search results below for current, factual sub-fields such as attestation steps, agencies, fees, and official links; cite the source URLs when you include factual claims):\n\n")

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

// isHagueMember reports whether the destination space is a Hague Convention
// member per the attestation_guide.hague_members system_setting (§97 Hague
// Convention section). The setting is a JSON array of space IDs. Returns false
// on any error (fail-safe: no apostille required if lookup fails).
func (s *Service) isHagueMember(ctx context.Context, destSpaceID pgtype.UUID) bool {
	if s.settings == nil || !destSpaceID.Valid {
		return false
	}

	raw := s.settings.GetStringOr(ctx, "attestation_guide.hague_members", "[]")

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
func (s *Service) List(ctx context.Context, destSpaceID, issuingSpaceID pgtype.UUID, documentType string, page, limit int32) ([]PathwayDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.ListPathways(ctx, ListPathwaysFilter{

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
func (s *Service) GetByID(ctx context.Context, id pgtype.UUID) (PathwayDTO, error) {
	row, err := s.store.GetPublishedPathwayByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PathwayDTO{}, pgx.ErrNoRows
		}

		return PathwayDTO{}, fmt.Errorf("attestationguide: get by id: %w", err)
	}

	return pathwayRowToDTO(row), nil
}

// Create creates a new attestation pathway. Admin-only
// (attestation_guide.manage permission checked at route level).
func (s *Service) Create(ctx context.Context, input CreatePathwayInput) (PathwayDTO, error) {
	if input.DocumentType == "" {
		return PathwayDTO{}, errors.New("document_type is required")
	}

	if input.IssuingSpaceID == "" {
		return PathwayDTO{}, errors.New("issuing_space_id is required")
	}

	if input.DestinationSpaceID == "" {
		return PathwayDTO{}, errors.New("destination_space_id is required")
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
		apostilleRequired = s.isHagueMember(ctx, destSpaceID)
	}

	p := Pathway{

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

	row, err := s.store.GetPathwayByID(ctx, created.ID)
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
func (s *Service) GenerateDraft(ctx context.Context, input GeneratePathwayInput) (PathwayDTO, error) {
	if input.DocumentType == "" || input.IssuingSpaceID == "" || input.DestinationSpaceID == "" {
		return PathwayDTO{}, errors.New("document_type, issuing_space_id, and destination_space_id are required")
	}

	var issuingSpaceID pgtype.UUID
	if err := issuingSpaceID.Scan(input.IssuingSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("invalid issuing_space_id: %w", err)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	// Look up the issuing and destination jurisdiction names so the prompt
	// and research queries are human-readable. Missing names are non-fatal;
	// we fall back to the supplied IDs.
	issuingName, _ := s.store.GetSpaceName(ctx, issuingSpaceID)
	destName, _ := s.store.GetSpaceName(ctx, destSpaceID)

	issuing := issuingName
	if issuing == "" {
		issuing = input.IssuingSpaceID
	}

	dest := destName
	if dest == "" {
		dest = input.DestinationSpaceID
	}

	// Build the generation prompt.
	prompt := input.Prompt
	if prompt == "" {
		prompt = fmt.Sprintf(
			"Generate an attestation pathway for document type '%s' from %s to %s. Include the sequence of steps, agencies, fees, turnaround times, and official links.\n\n"+
				"Use the GROUNDED RESEARCH section below for current attestation requirements. Cite the source URLs when you make factual claims. If no grounded research is available, use your training knowledge and clearly note that details may need verification.",
			input.DocumentType, issuing, dest,
		)
	} else {
		prompt = fmt.Sprintf(
			"%s\n\nContext: document_type=%s, issuing_space=%s, destination_space=%s. "+
				"Use the GROUNDED RESEARCH section below for current attestation requirements. Cite the source URLs when you make factual claims.",
			prompt, input.DocumentType, issuing, dest,
		)
	}

	// Augment the prompt with grounded web research. If research fails, log
	// a warning and continue with the original prompt (graceful degradation).
	if s.researcher != nil {
		researchText, _, researchErr := s.researcher.Research(ctx, destSpaceID, issuingSpaceID, input.DocumentType)
		if researchErr != nil {
			slog.Warn("attestationguide: web research failed, continuing with original prompt", "error", researchErr)
		} else if researchText != "" {
			prompt = prompt + "\n\n" + researchText
		}
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
				"prompt": prompt,
			},
		})
		if derr != nil {
			return PathwayDTO{}, fmt.Errorf("attestationguide: AI generation failed: %w", derr)
		}

		pathwaySteps = parsePathwayStepsFromAIResponse(resp)
	}

	p := Pathway{

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

	row, err := s.store.GetPathwayByID(ctx, created.ID)
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
func (s *Service) Approve(ctx context.Context, id, reviewerID pgtype.UUID, notes string) (PathwayDTO, error) {
	// Load the pathway to determine the destination country for the Hague
	// Convention lookup (BUG-002/BUG-003).
	row, err := s.store.GetPathwayByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PathwayDTO{}, pgx.ErrNoRows
		}

		return PathwayDTO{}, fmt.Errorf("attestationguide: approve: lookup: %w", err)
	}

	apostilleRequired := s.isHagueMember(ctx, row.DestinationSpaceID)

	var notesPtr *string
	if notes != "" {
		notesPtr = &notes
	}

	if err := s.store.ApprovePathway(ctx, id, reviewerID, apostilleRequired, notesPtr); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PathwayDTO{}, pgx.ErrNoRows
		}

		return PathwayDTO{}, fmt.Errorf("attestationguide: approve: %w", err)
	}

	row, err = s.store.GetPathwayByID(ctx, id)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("attestationguide: approve: reload: %w", err)
	}

	return pathwayRowToDTO(row), nil
}

// SubmitCommunityUpdate creates a pending community update for a
// pathway. Auth required (attestation_guide.update permission). Only
// published, active pathways accept community updates (BUG-002).
func (s *Service) SubmitCommunityUpdate(ctx context.Context, pathwayID, userID pgtype.UUID, input CreateCommunityUpdateInput) (CommunityUpdateDTO, error) {
	// Verify the pathway exists, belongs to the platform, and is published +
	// active (BUG-002: no updates on draft/ai_draft/pending/rejected pathways).
	if _, err := s.store.GetPublishedPathwayByID(ctx, pathwayID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
func (s *Service) ReviewCommunityUpdate(ctx context.Context, updateID, reviewerID pgtype.UUID, approve bool) (CommunityUpdateDTO, error) {
	status := UpdateStatusApproved
	if !approve {
		status = UpdateStatusRejected
	}

	if err := s.store.ReviewCommunityUpdate(ctx, updateID, reviewerID, status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CommunityUpdateDTO{}, pgx.ErrNoRows
		}

		return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: review update: %w", err)
	}
	// Reload the full row so the DTO includes all fields (BUG-005 fix).
	row, err := s.store.GetCommunityUpdateByID(ctx, updateID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CommunityUpdateDTO{}, pgx.ErrNoRows
		}

		return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: review update: reload: %w", err)
	}

	// BUG-003 + BUG-004: on approval, apply the update to the parent pathway
	// (patch pathway_steps + bump community_last_verified).
	if approve && row.PathwayID.Valid {
		if err := s.store.ApplyApprovedUpdateToPathway(ctx, row.PathwayID, row.StepIndex, row.FeeEstimate, row.TurnaroundDays); err != nil {
			return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: review update: apply: %w", err)
		}
	}

	return communityUpdateToDTO(row), nil
}

// AdminList returns all attestation pathways (any status) for the
// moderation queue. Admin-only (attestation_guide.manage permission).
// Supports search (q) and sort (sortBy/sortDir) per BUG-004.
func (s *Service) AdminList(ctx context.Context, destSpaceID pgtype.UUID, status, documentType string, isActive *bool, search, sortBy, sortDir string, page, limit int32) ([]PathwayDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.AdminList(ctx, AdminListFilter{

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
func (s *Service) AdminListUpdates(ctx context.Context, status string, page, limit int32) ([]CommunityUpdateDTO, int64, error) {
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

		Status: status,
		Limit:  limit,
		Offset: offset,
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
