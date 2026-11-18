// Package attestationguide — service.go orchestrates attestation
// pathway lookup, creation, HITL approval, and community update
// moderation. Pathways are curated guides for document
// legalization/apostille across jurisdictions. Feature-flagged via the
// attestation_guide.enabled system_setting (§97, ADR-105).
// // Ref: japaconnect-engineering/sections/97-attestation-guide.md
// //	japaconnect-engineering/adr/ADR-105-attestation-guide.md
package attestationguide

import (
	"bytes"
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

// ErrInvalidInput marks request-validation failures so handlers can map them
// to 400 instead of 500.
var ErrInvalidInput = errors.New("invalid input")

// isJSONArray reports whether b is a JSON array (pathway_steps' shape) —
// json.Valid alone would accept a scalar, null, or object that later step
// renderers and step_index bounds checks cannot consume.
func isJSONArray(b []byte) bool {
	t := bytes.TrimSpace(b)
	return len(t) > 0 && t[0] == '[' && json.Valid(t)
}

// isValidDocumentType reports whether v is a §97 canonical document type.
func isValidDocumentType(v string) bool {
	switch v {
	case DocumentTypeBirthCertificate, DocumentTypeDegree, DocumentTypeAcademicTranscript,
		DocumentTypeMarriage, DocumentTypeDeathCertificate, DocumentTypeDivorceCertificate,
		DocumentTypeSingleStatus, DocumentTypeAdoptionPapers, DocumentTypePoliceClearance,
		DocumentTypeProfessionalLicence, DocumentTypePowerOfAttorney, DocumentTypeDrivingLicence,
		DocumentTypeEmploymentLetter, DocumentTypeCompanyRegistration, DocumentTypeMedicalCertificate,
		DocumentTypeOther:
		return true
	}

	return false
}

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
	GuideSlug             string          `json:"guide_slug,omitempty"`
	CreatedAt             string          `json:"created_at"`
	UpdatedAt             string          `json:"updated_at"`
}

// scrubInternal clears fields that are internal provenance/audit data —
// the reviewer's identity, review notes, and the prompt hash must not be
// exposed on the anonymous public endpoints. Admin surfaces keep them.
func (d *PathwayDTO) scrubInternal() {
	d.HITLReviewedBy = ""
	d.HITLReviewedAt = ""
	d.HITLReviewNotes = ""
	d.AIPromptHash = ""
}

// CommunityUpdateDTO is the API response shape for a community update.
// The Pathway*/SubmitterName fields give the admin moderation queue enough
// context to review a submission inline (populated by AdminListUpdates).
type CommunityUpdateDTO struct {
	ID                   string `json:"id"`
	PathwayID            string `json:"pathway_id"`
	PathwayDocumentType  string `json:"pathway_document_type,omitempty"`
	IssuingSpaceName     string `json:"issuing_space_name,omitempty"`
	DestinationSpaceName string `json:"destination_space_name,omitempty"`
	UserID               string `json:"user_id"`
	SubmitterName        string `json:"submitter_name,omitempty"`
	ReviewerName         string `json:"reviewer_name,omitempty"`
	StepIndex            *int32 `json:"step_index,omitempty"`
	FeeEstimate          string `json:"fee_estimate,omitempty"`
	TurnaroundDays       *int32 `json:"turnaround_days,omitempty"`
	Note                 string `json:"note,omitempty"`
	Status               string `json:"status"`
	ReviewedBy           string `json:"reviewed_by,omitempty"`
	CreatedAt            string `json:"created_at"`
}

// CreatePathwayInput is the request body for POST /admin/attestation.
type CreatePathwayInput struct {
	DocumentType          string          `json:"document_type"`
	IssuingSpaceID        string          `json:"issuing_space_id"`
	DestinationSpaceID    string          `json:"destination_space_id"`
	ApostilleRequired     *bool           `json:"apostille_required"`
	PathwaySteps          json.RawMessage `json:"pathway_steps"`
	CommunityLastVerified string          `json:"community_last_verified"`
	Status                string          `json:"status"`
	IsActive              *bool           `json:"is_active"`
	AIGenerated           *bool           `json:"ai_generated"`
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

// UpdatePathwayInput carries optional PATCH fields for
// PATCH /admin/attestation/{id}. Nil fields are left untouched; this is the
// lifecycle-management surface (edit steps/fields, reject, deactivate) that
// the approve-only flow cannot express.
type UpdatePathwayInput struct {
	DocumentType          *string         `json:"document_type"`
	IssuingSpaceID        *string         `json:"issuing_space_id"`
	DestinationSpaceID    *string         `json:"destination_space_id"`
	ApostilleRequired     *bool           `json:"apostille_required"`
	PathwaySteps          json.RawMessage `json:"pathway_steps"`
	CommunityLastVerified *string         `json:"community_last_verified"`
	Status                *string         `json:"status"`
	IsActive              *bool           `json:"is_active"`
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

// isHagueMember reports whether a space is a Hague Convention member per the
// attestation_guide.hague_members system_setting (§97 Hague Convention
// section). The setting is a JSON array of space IDs. Returns false on any
// error (fail-safe: no apostille required if lookup fails).
func (s *Service) isHagueMember(ctx context.Context, spaceID pgtype.UUID) bool {
	if s.settings == nil || !spaceID.Valid {
		return false
	}

	raw := s.settings.GetStringOr(ctx, "attestation_guide.hague_members", "[]")

	var members []string

	if err := json.Unmarshal([]byte(raw), &members); err != nil {
		return false
	}

	id := spaceID.String()
	for _, m := range members {
		if m == id {
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
		d := pathwayRowToDTO(r)
		d.scrubInternal()
		dtos = append(dtos, d)
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

	dto := pathwayRowToDTO(row)
	dto.scrubInternal()

	// Enrich with the linked editorial guide slug — matched by explicit
	// field_values.pathway_id or by the document_type + corridor geo-code
	// combination — so the public detail page can cross-link tool→guide.
	// Enrichment failure never fails the read — the page falls back to
	// the guides hub.
	if slug, slugErr := s.store.GuideSlugForPathway(ctx, row); slugErr != nil {
		slog.Warn("attestationguide: guide slug lookup failed", "id", id, "error", slugErr)
	} else {
		dto.GuideSlug = slug
	}

	return dto, nil
}

// Create creates a new attestation pathway. Admin-only
// (attestation_guide.manage permission checked at route level).
func (s *Service) Create(ctx context.Context, input CreatePathwayInput) (PathwayDTO, error) {
	if input.DocumentType == "" {
		return PathwayDTO{}, fmt.Errorf("%w: document_type is required", ErrInvalidInput)
	}

	if !isValidDocumentType(input.DocumentType) {
		return PathwayDTO{}, fmt.Errorf("%w: invalid document_type %q", ErrInvalidInput, input.DocumentType)
	}

	if input.IssuingSpaceID == "" {
		return PathwayDTO{}, fmt.Errorf("%w: issuing_space_id is required", ErrInvalidInput)
	}

	if input.DestinationSpaceID == "" {
		return PathwayDTO{}, fmt.Errorf("%w: destination_space_id is required", ErrInvalidInput)
	}

	var issuingSpaceID pgtype.UUID
	if err := issuingSpaceID.Scan(input.IssuingSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("%w: invalid issuing_space_id: %v", ErrInvalidInput, err)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("%w: invalid destination_space_id: %v", ErrInvalidInput, err)
	}

	steps := []byte(input.PathwaySteps)
	if len(steps) == 0 {
		steps = []byte("[]")
	} else if !isJSONArray(steps) {
		return PathwayDTO{}, fmt.Errorf("%w: pathway_steps must be a JSON array", ErrInvalidInput)
	}

	// Both spaces must exist (and not be soft-deleted) — a bad ID would
	// otherwise surface as an FK-violation 500 at insert time.
	names, err := s.store.GetSpaceNames(ctx, []pgtype.UUID{issuingSpaceID, destSpaceID})
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("attestationguide: create: space lookup: %w", err)
	}

	if names[issuingSpaceID] == "" {
		return PathwayDTO{}, fmt.Errorf("%w: issuing_space_id not found", ErrInvalidInput)
	}

	if names[destSpaceID] == "" {
		return PathwayDTO{}, fmt.Errorf("%w: destination_space_id not found", ErrInvalidInput)
	}

	// A pathway where the issuing and destination jurisdictions are the
	// same is meaningless — attestation is only ever cross-border.
	if issuingSpaceID == destSpaceID {
		return PathwayDTO{}, fmt.Errorf("%w: issuing and destination spaces must differ", ErrInvalidInput)
	}

	// §97 Hague Convention: an apostille is only usable when BOTH the issuing
	// and destination jurisdictions are Convention members — a member
	// destination cannot accept an apostille from a non-member issuer, and a
	// member issuer's apostille has no effect in a non-member destination.
	// Note: §97's text keys the lookup on destination alone; the both-sides
	// check is the correct semantic (flagged in the §97 audit).
	apostilleRequired := s.isHagueMember(ctx, destSpaceID) && s.isHagueMember(ctx, issuingSpaceID)
	if input.ApostilleRequired != nil {
		apostilleRequired = *input.ApostilleRequired
	}

	if input.Status != "" {
		switch input.Status {
		case PathwayStatusDraft, PathwayStatusAIDraft, PathwayStatusPending,
			PathwayStatusPublished, PathwayStatusRejected:
		default:
			return PathwayDTO{}, fmt.Errorf("%w: invalid status %s", ErrInvalidInput, input.Status)
		}
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

	// ai_generated stamps provenance when the agent-tool create surface is
	// used (ADR-088 clamp forces status='ai_draft' + is_active=false upstream).
	if input.AIGenerated != nil {
		p.AIGenerated = *input.AIGenerated
	}

	// ADR-088: an ai_generated pathway may never be created straight into
	// 'published' — it must land as a draft and pass through the approve
	// endpoint so hitl_reviewed_by/at is stamped. The agent clamp already
	// enforces this on the tool surface; this guards the direct admin API.
	if p.AIGenerated && (p.Status == "" || p.Status == PathwayStatusPublished) {
		return PathwayDTO{}, fmt.Errorf("%w: ai_generated pathways cannot be created with status 'published' — approve via the review flow", ErrInvalidInput)
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
			return PathwayDTO{}, fmt.Errorf("%w: invalid community_last_verified: %v", ErrInvalidInput, err)
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
		return PathwayDTO{}, fmt.Errorf("%w: document_type, issuing_space_id, and destination_space_id are required", ErrInvalidInput)
	}

	if !isValidDocumentType(input.DocumentType) {
		return PathwayDTO{}, fmt.Errorf("%w: invalid document_type %q", ErrInvalidInput, input.DocumentType)
	}

	var issuingSpaceID pgtype.UUID
	if err := issuingSpaceID.Scan(input.IssuingSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("%w: invalid issuing_space_id: %v", ErrInvalidInput, err)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("%w: invalid destination_space_id: %v", ErrInvalidInput, err)
	}

	// Look up the issuing and destination jurisdiction names so the prompt
	// and research queries are human-readable. The lookup also validates
	// the spaces exist — a bad ID would otherwise surface as an FK-violation
	// 500 at insert time.
	names, err := s.store.GetSpaceNames(ctx, []pgtype.UUID{issuingSpaceID, destSpaceID})
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("attestationguide: generate draft: space lookup: %w", err)
	}

	issuingName, destName := names[issuingSpaceID], names[destSpaceID]

	if issuingName == "" {
		return PathwayDTO{}, fmt.Errorf("%w: issuing_space_id not found", ErrInvalidInput)
	}

	if destName == "" {
		return PathwayDTO{}, fmt.Errorf("%w: destination_space_id not found", ErrInvalidInput)
	}

	if issuingSpaceID == destSpaceID {
		return PathwayDTO{}, fmt.Errorf("%w: issuing and destination spaces must differ", ErrInvalidInput)
	}

	// A custom prompt rides into the AI Gateway payload — cap it so a
	// malformed or abusive body cannot send an unbounded (and unbounded-cost)
	// prompt upstream.
	if len(input.Prompt) > 8000 {
		return PathwayDTO{}, fmt.Errorf("%w: prompt must be 8000 characters or fewer", ErrInvalidInput)
	}

	// Build the generation prompt.
	prompt := input.Prompt
	if prompt == "" {
		prompt = fmt.Sprintf(
			"Generate an attestation pathway for document type '%s' from %s to %s. Include the sequence of steps, agencies, fees, turnaround times, and official links.\n\n"+
				"Use the GROUNDED RESEARCH section below for current attestation requirements. Cite the source URLs when you make factual claims. If no grounded research is available, use your training knowledge and clearly note that details may need verification.",
			input.DocumentType, issuingName, destName,
		)
	} else {
		prompt = fmt.Sprintf(
			"%s\n\nContext: document_type=%s, issuing_space=%s, destination_space=%s. "+
				"Use the GROUNDED RESEARCH section below for current attestation requirements. Cite the source URLs when you make factual claims.",
			prompt, input.DocumentType, issuingName, destName,
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

		// Record the provider-reported model over the generic marker when
		// the gateway surfaces it — provenance should name the real model.
		if m := ai.ExtractModel(resp); m != "" {
			aiModel = m
		}
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
// gateway response. Provider output goes through ai.ExtractText so every
// response shape (plain text/content/output keys AND the OpenAI-compatible
// choices[0].message.content structure) is covered — probing Raw keys
// directly silently returned empty steps for OpenAI-compatible providers.
// If the model returned a JSON array (as the prompt asks), it is used
// verbatim; otherwise the text is wrapped into a single-step array for the
// reviewer to refine.
func parsePathwayStepsFromAIResponse(resp ai.Response) []byte {
	text := ai.ExtractText(resp)
	if text == "" {
		return []byte("[]")
	}

	// Strip a markdown code fence if the model wrapped the JSON in one.
	trimmed := strings.TrimSpace(text)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")

	if isJSONArray([]byte(strings.TrimSpace(trimmed))) {
		return json.RawMessage(strings.TrimSpace(trimmed))
	}

	wrapped, _ := json.Marshal([]map[string]string{{"step": text}})

	return wrapped
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

	// Approve is the HITL publish act for draft/queued/rejected pathways.
	// An already-published row must not be re-approved — it would overwrite
	// hitl_reviewed_by/at and notes for a review that never happened.
	if row.Status == PathwayStatusPublished {
		return PathwayDTO{}, fmt.Errorf("%w: pathway is already published", ErrInvalidInput)
	}

	// Apostille is valid only when issuing AND destination are both Hague
	// Convention members (see Create for the both-sides rationale).
	apostilleRequired := s.isHagueMember(ctx, row.DestinationSpaceID) && s.isHagueMember(ctx, row.IssuingSpaceID)

	var notesPtr *string
	if notes != "" {
		notesPtr = &notes
	}

	if err := s.store.ApprovePathway(ctx, id, reviewerID, apostilleRequired, notesPtr); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The WHERE's status predicate lost a concurrent-approval race:
			// the row exists but was just published by the other request.
			// 400 "already published" is truer than 404.
			return PathwayDTO{}, fmt.Errorf("%w: pathway is already published", ErrInvalidInput)
		}

		return PathwayDTO{}, fmt.Errorf("attestationguide: approve: %w", err)
	}

	row, err = s.store.GetPathwayByID(ctx, id)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("attestationguide: approve: reload: %w", err)
	}

	return pathwayRowToDTO(row), nil
}

// AdminUpdatePathway applies a partial admin patch to a pathway (PATCH
// /admin/attestation/{id}). This is the lifecycle surface for editing steps,
// correcting fields, rejecting, or deactivating a pathway — operations the
// approve-only flow cannot express. Admin-only
// (attestation_guide.manage permission checked at route level).
//
// Named AdminUpdatePathway (not Update) deliberately: the ai_tools writer
// resolves an "Update" method reflectively, and the seeded
// attestation_guide.update/.assign_geo agent tools are disabled (000788) —
// a method named Update would silently light up a disabled op.
func (s *Service) AdminUpdatePathway(ctx context.Context, id pgtype.UUID, input UpdatePathwayInput) (PathwayDTO, error) {
	// The merge runs inside MutatePathway's FOR UPDATE transaction — two
	// concurrent PATCHes serialize instead of silently losing one set of
	// changes (UpdatePathway writes the full row).
	err := s.store.MutatePathway(ctx, id, func(p *Pathway) error {
		return s.mergePathwayPatch(ctx, p, input)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PathwayDTO{}, pgx.ErrNoRows
		}

		if errors.Is(err, ErrInvalidInput) {
			return PathwayDTO{}, err
		}

		return PathwayDTO{}, fmt.Errorf("attestationguide: update: %w", err)
	}

	row, err := s.store.GetPathwayByID(ctx, id)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("attestationguide: update: reload: %w", err)
	}

	return pathwayRowToDTO(row), nil
}

// mergePathwayPatch overlays the PATCH fields onto a locked pathway row.
// Runs inside MutatePathway's transaction; returning ErrInvalidInput rolls
// back cleanly.
func (s *Service) mergePathwayPatch(ctx context.Context, p *Pathway, input UpdatePathwayInput) error {

	if input.DocumentType != nil {
		if !isValidDocumentType(*input.DocumentType) {
			return fmt.Errorf("%w: invalid document_type %q", ErrInvalidInput, *input.DocumentType)
		}

		p.DocumentType = *input.DocumentType
	}

	if input.IssuingSpaceID != nil || input.DestinationSpaceID != nil {
		// A patched space must exist (and not be soft-deleted) — a bad ID
		// would otherwise surface as an FK-violation 500 at write time.
		var (
			issuingID pgtype.UUID
			destID    pgtype.UUID
		)

		if input.IssuingSpaceID != nil {
			if err := issuingID.Scan(*input.IssuingSpaceID); err != nil {
				return fmt.Errorf("%w: invalid issuing_space_id: %v", ErrInvalidInput, err)
			}
		} else {
			issuingID = p.IssuingSpaceID
		}

		if input.DestinationSpaceID != nil {
			if err := destID.Scan(*input.DestinationSpaceID); err != nil {
				return fmt.Errorf("%w: invalid destination_space_id: %v", ErrInvalidInput, err)
			}
		} else {
			destID = p.DestinationSpaceID
		}

		names, err := s.store.GetSpaceNames(ctx, []pgtype.UUID{issuingID, destID})
		if err != nil {
			return fmt.Errorf("attestationguide: update: space lookup: %w", err)
		}

		if names[issuingID] == "" {
			return fmt.Errorf("%w: issuing_space_id not found", ErrInvalidInput)
		}

		if names[destID] == "" {
			return fmt.Errorf("%w: destination_space_id not found", ErrInvalidInput)
		}

		if issuingID == destID {
			return fmt.Errorf("%w: issuing and destination spaces must differ", ErrInvalidInput)
		}

		p.IssuingSpaceID = issuingID
		p.DestinationSpaceID = destID

		// apostille_required is derived from the jurisdiction pair — refresh
		// it when either space changes unless the patch sets it explicitly.
		if input.ApostilleRequired == nil {
			p.ApostilleRequired = s.isHagueMember(ctx, destID) && s.isHagueMember(ctx, issuingID)
		}
	}

	if input.ApostilleRequired != nil {
		p.ApostilleRequired = *input.ApostilleRequired
	}

	if input.PathwaySteps != nil {
		if !isJSONArray(input.PathwaySteps) {
			return fmt.Errorf("%w: pathway_steps must be a JSON array", ErrInvalidInput)
		}

		p.PathwaySteps = []byte(input.PathwaySteps)
	}

	if input.CommunityLastVerified != nil {
		if *input.CommunityLastVerified == "" {
			p.CommunityLastVerified = pgtype.Date{}
		} else if err := p.CommunityLastVerified.Scan(*input.CommunityLastVerified); err != nil {
			return fmt.Errorf("%w: invalid community_last_verified: %v", ErrInvalidInput, err)
		}
	}

	wasPublished := p.Status == PathwayStatusPublished

	if input.Status != nil {
		switch *input.Status {
		case PathwayStatusDraft, PathwayStatusAIDraft, PathwayStatusPending,
			PathwayStatusPublished, PathwayStatusRejected:
			p.Status = *input.Status
		default:
			return fmt.Errorf("%w: invalid status %s", ErrInvalidInput, *input.Status)
		}
	}

	if input.IsActive != nil {
		p.IsActive = *input.IsActive
	}

	// ADR-088: transitioning an unreviewed AI-generated pathway into
	// 'published' must go through the approve endpoint so
	// hitl_reviewed_by/at is stamped — a PATCH bypass would put AI content
	// live with no recorded review and, because approve rejects
	// already-published rows, would wedge the pathway. Guarded on the
	// transition only: editing or deactivating a row already published
	// before the guard existed stays possible, and rows already stamped
	// (e.g. rejected-then-republished) keep provenance and move freely.
	if !wasPublished && p.Status == PathwayStatusPublished && p.AIGenerated && !p.HITLReviewedBy.Valid {
		return fmt.Errorf("%w: AI-generated pathways must be published via the approve endpoint", ErrInvalidInput)
	}

	return nil
}

// SubmitCommunityUpdate creates a pending community update for a
// pathway. Auth required (attestation_guide.update permission). Only
// published, active pathways accept community updates (BUG-002).
func (s *Service) SubmitCommunityUpdate(ctx context.Context, pathwayID, userID pgtype.UUID, input CreateCommunityUpdateInput) (CommunityUpdateDTO, error) {
	// Verify the pathway exists, belongs to the platform, and is published +
	// active (BUG-002: no updates on draft/ai_draft/pending/rejected pathways).
	pathway, err := s.store.GetPublishedPathwayByID(ctx, pathwayID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CommunityUpdateDTO{}, pgx.ErrNoRows
		}

		return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: submit update: lookup pathway: %w", err)
	}

	// Content validation: at least one substantive field is required, and
	// free-text fields are capped so a submission always carries reviewable
	// information without allowing unbounded payloads.
	fee := strings.TrimSpace(input.FeeEstimate)
	note := strings.TrimSpace(input.Note)

	// step_index is a locator for the submission, not content by itself — a
	// reviewable update always carries a fee, turnaround, or note.
	if fee == "" && input.TurnaroundDays == nil && note == "" {
		return CommunityUpdateDTO{}, fmt.Errorf("%w: at least one of fee_estimate, turnaround_days, or note is required", ErrInvalidInput)
	}

	if len(fee) > 100 {
		return CommunityUpdateDTO{}, fmt.Errorf("%w: fee_estimate must be 100 characters or fewer", ErrInvalidInput)
	}

	if len(note) > 2000 {
		return CommunityUpdateDTO{}, fmt.Errorf("%w: note must be 2000 characters or fewer", ErrInvalidInput)
	}

	if input.TurnaroundDays != nil && (*input.TurnaroundDays < 0 || *input.TurnaroundDays > 36500) {
		return CommunityUpdateDTO{}, fmt.Errorf("%w: turnaround_days must be between 0 and 36500", ErrInvalidInput)
	}

	// step_index is 0-based into pathway_steps; validate it is in range so a
	// later approval can always apply it.
	if input.StepIndex != nil {
		var steps []json.RawMessage
		if len(pathway.PathwaySteps) > 0 {
			_ = json.Unmarshal(pathway.PathwaySteps, &steps)
		}

		if *input.StepIndex < 0 || int(*input.StepIndex) >= len(steps) {
			return CommunityUpdateDTO{}, fmt.Errorf("%w: step_index out of range (pathway has %d steps)", ErrInvalidInput, len(steps))
		}
	}

	u := CommunityUpdate{
		PathwayID:      pathwayID,
		UserID:         userID,
		StepIndex:      input.StepIndex,
		TurnaroundDays: input.TurnaroundDays,
		Status:         UpdateStatusPending,
	}
	if fee != "" {
		u.FeeEstimate = &fee
	}

	if note != "" {
		u.Note = &note
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

	// Load the pending row first: only a pending update may transition —
	// re-approving would re-apply the patch and overwrite the original
	// reviewer, and rejecting an already-approved update would mark it
	// rejected while the patch stays applied to the pathway.
	pending, err := s.store.GetCommunityUpdateByID(ctx, updateID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CommunityUpdateDTO{}, pgx.ErrNoRows
		}

		return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: review update: preload: %w", err)
	}

	if pending.Status != UpdateStatusPending {
		return CommunityUpdateDTO{}, fmt.Errorf("%w: update is already %s", ErrInvalidInput, pending.Status)
	}

	// On approval, validate the step index is still in range BEFORE the
	// apply — a stale index must fail here or the pathway gets a verified
	// bump on a patch that never landed.
	if approve {
		if pending.StepIndex != nil && pending.PathwayID.Valid {
			pw, err := s.store.GetPathwayByID(ctx, pending.PathwayID)
			if err != nil {
				return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: review update: pathway lookup: %w", err)
			}

			var steps []json.RawMessage
			if len(pw.Pathway.PathwaySteps) > 0 {
				_ = json.Unmarshal(pw.Pathway.PathwaySteps, &steps)
			}

			if *pending.StepIndex < 0 || int(*pending.StepIndex) >= len(steps) {
				return CommunityUpdateDTO{}, fmt.Errorf("%w: step_index out of range (pathway has %d steps)", ErrInvalidInput, len(steps))
			}
		}

		// Apply BEFORE flipping the row to approved: if the apply fails the
		// row stays pending and the review can be retried. A status-write
		// failure instead leaves an applied-but-pending update that a retry
		// simply re-applies (the patch writes absolute values, so it is
		// idempotent).
		if pending.PathwayID.Valid {
			if err := s.store.ApplyApprovedUpdateToPathway(ctx, pending.PathwayID, pending.StepIndex, pending.FeeEstimate, pending.TurnaroundDays); err != nil {
				return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: review update: apply: %w", err)
			}
		}
	}

	if err := s.store.ReviewCommunityUpdate(ctx, updateID, reviewerID, status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The WHERE's u.status='pending' predicate lost a concurrent
			// review race — the row exists but was already decided.
			return CommunityUpdateDTO{}, fmt.Errorf("%w: update was already reviewed", ErrInvalidInput)
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

	// Default to the pending queue; the explicit "all" sentinel returns every
	// status so admins can audit past decisions.
	if status == "" {
		status = UpdateStatusPending
	} else if status == "all" {
		status = ""
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

		PathwayDocumentType: u.PathwayDocumentType,
	}
	if u.IssuingSpaceName != nil {
		dto.IssuingSpaceName = *u.IssuingSpaceName
	}

	if u.DestinationSpaceName != nil {
		dto.DestinationSpaceName = *u.DestinationSpaceName
	}

	if u.SubmitterName != nil {
		dto.SubmitterName = *u.SubmitterName
	}

	if u.ReviewerName != nil {
		dto.ReviewerName = *u.ReviewerName
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
