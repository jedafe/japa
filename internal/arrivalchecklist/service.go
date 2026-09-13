// Package arrivalchecklist — service.go orchestrates arrival checklist
// guide creation, update, HITL approval, public selector browsing, and
// guide item retrieval. It manages the arrival_checklist_guides and
// arrival_checklist_items tables via the local Store. Feature-flagged via
// the arrival_checklist.enabled system_setting (§92, ADR-100).
// // Ref: japaconnect-engineering/sections/92-arrival-checklist.md
// //	japaconnect-engineering/adr/ADR-100-arrival-checklist.md
package arrivalchecklist

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

// ErrPublishViaUpdate is returned when UpdateGuide is called with
// status='published'. Publishing must go through ApproveGuide to enforce
// the ADR-088 HITL review gate (hitl_reviewed_by + hitl_reviewed_at).
var ErrPublishViaUpdate = errors.New("arrivalchecklist: cannot publish via update; use the approve endpoint")

// AIDispatcher is the subset of the AI Gateway the arrival checklist
// needs to generate draft guides. *ai.Dispatcher satisfies this.
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}

// Researcher runs grounded web research for a destination and visa category.
// *Service implements this interface, but it can be swapped in tests.
type Researcher interface {
	Research(ctx context.Context, tenantID, destSpaceID, citySpaceID pgtype.UUID, visaCategory string) (string, []string, error)
}

// GuideDTO is the API response shape for a single arrival checklist guide.
type GuideDTO struct {
	ID                 string `json:"id"`
	DestinationSpaceID string `json:"destination_space_id"`
	DestSpaceName      string `json:"destination_space_name,omitempty"`
	DestSpaceCode      string `json:"destination_space_code,omitempty"`
	VisaCategory       string `json:"visa_category,omitempty"`
	ApplicantType      string `json:"applicant_type"`
	Version            int32  `json:"version"`
	Status             string `json:"status"`
	IsActive           bool   `json:"is_active"`
	AIGenerated        bool   `json:"ai_generated"`
	AIModel            string `json:"ai_model,omitempty"`
	AIPromptHash       string `json:"ai_prompt_hash,omitempty"`
	HITLReviewedBy     string `json:"hitl_reviewed_by,omitempty"`
	HITLReviewedByName string `json:"hitl_reviewed_by_name,omitempty"`
	HITLReviewedAt     string `json:"hitl_reviewed_at,omitempty"`
	HITLReviewNotes    string `json:"hitl_review_notes,omitempty"`
	IsJapabotIndexed   bool   `json:"is_japabot_indexed"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

// ChecklistItemDTO is the API response shape for a single checklist item.
type ChecklistItemDTO struct {
	ID                    string `json:"id"`
	GuideID               string `json:"guide_id"`
	Phase                 string `json:"phase"`
	Title                 string `json:"title"`
	Description           string `json:"description,omitempty"`
	OfficialLink          string `json:"official_link,omitempty"`
	OriginCountryModifier any    `json:"origin_country_modifier,omitempty"`
	SortOrder             int32  `json:"sort_order"`
	IsFlagged             bool   `json:"is_flagged"`
	CreatedAt             string `json:"created_at"`
}

// SelectorDTO is the API response shape for a selector entry (distinct
// destination + visa category combination). Includes GuideID so the
// frontend can fetch items via GET /arrival-checklist/:guide_id without
// a separate list-guides call.
type SelectorDTO struct {
	DestinationSpaceID string `json:"destination_space_id"`
	DestSpaceName      string `json:"destination_space_name,omitempty"`
	DestSpaceCode      string `json:"destination_space_code,omitempty"`
	VisaCategory       string `json:"visa_category,omitempty"`
	GuideCount         int64  `json:"guide_count"`
	GuideID            string `json:"guide_id"`
}

// CreateGuideRequest is the request body for POST /admin/arrival-checklists.
type CreateGuideRequest struct {
	DestinationSpaceID string `json:"destination_space_id"`
	VisaCategory       string `json:"visa_category"`
	ApplicantType      string `json:"applicant_type"`
	Version            int32  `json:"version"`
	Status             string `json:"status"`
	IsActive           *bool  `json:"is_active"`
}

// UpdateGuideRequest is the request body for PATCH /admin/arrival-checklists/:id.
type UpdateGuideRequest struct {
	DestinationSpaceID *string `json:"destination_space_id"`
	VisaCategory       *string `json:"visa_category"`
	ApplicantType      *string `json:"applicant_type"`
	Version            *int32  `json:"version"`
	Status             *string `json:"status"`
	IsActive           *bool   `json:"is_active"`
	IsJapabotIndexed   *bool   `json:"is_japabot_indexed"`
	HITLReviewNotes    *string `json:"hitl_review_notes"`
}

// ApproveGuideRequest is the request body for PATCH
// /admin/arrival-checklists/:id/approve.
type ApproveGuideRequest struct {
	Notes *string `json:"notes"`
}

// GenerateGuideRequest is the request body for POST
// /admin/arrival-checklists/generate (Content Agent draft).
type GenerateGuideRequest struct {
	DestinationSpaceID string `json:"destination_space_id"`
	VisaCategory       string `json:"visa_category"`
	ApplicantType      string `json:"applicant_type"`
	Prompt             string `json:"prompt"`
}

// GenerateGuideResponse is the response for the generate endpoint. The
// draft is created in 'ai_draft' status and requires HITL approval
// (ApproveGuide) before publication. Ref: ADR-088, geo-aware-tool-pattern.md.
type GenerateGuideResponse struct {
	Status             string `json:"status"`
	Message            string `json:"message"`
	GuideID            string `json:"guide_id,omitempty"`
	DestinationSpaceID string `json:"destination_space_id,omitempty"`
	VisaCategory       string `json:"visa_category,omitempty"`
	ApplicantType      string `json:"applicant_type,omitempty"`
}

// Service holds the business logic for the Arrival Checklist Generator.
type Service struct {
	store      *Store
	settings   *config.Settings
	dispatcher AIDispatcher
	httpClient *http.Client // nil = http.DefaultClient
	researcher Researcher   // nil = use s.Research
}

// NewService creates an arrival checklist service.
func NewService(store *Store, settings *config.Settings) *Service {
	s := &Service{
		store:    store,
		settings: settings,
	}
	s.researcher = s

	return s
}

// SetDispatcher injects the AI Gateway dispatcher for draft generation
// (content_generation category). Optional: when nil or when the call
// fails, the draft is still created so a reviewer can fill it manually.
func (s *Service) SetDispatcher(d AIDispatcher) { s.dispatcher = d }

// SetHTTPClient sets the HTTP client used for external web research. When
// nil, http.DefaultClient is used.
func (s *Service) SetHTTPClient(c *http.Client) { s.httpClient = c }

// Enabled reads the arrival_checklist.enabled system_setting toggle
// (§92 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, pgtype.UUID{}, "arrival_checklist.enabled", false)
}

// Research runs grounded web research for the given destination and visa
// category. The citySpaceID parameter is reserved for future city-level
// refinement and is currently ignored. It returns a formatted "GROUNDED
// RESEARCH" section and a list of source URLs. Errors are returned so the
// caller can log and fall back.
func (s *Service) Research(ctx context.Context, tenantID, destSpaceID, citySpaceID pgtype.UUID, visaCategory string) (string, []string, error) {
	_ = citySpaceID // reserved for future city-level refinement

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

	visa := visaCategory
	if visa == "" {
		visa = "arrival"
	} else {
		visa = strings.ReplaceAll(visa, "_", " ")
	}

	queries := []string{
		fmt.Sprintf("current arrival requirements for %s visa in %s 2026", visa, dest),
		fmt.Sprintf("current documents required at immigration %s 2026", dest),
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

	sb.WriteString("GROUNDED RESEARCH (use the search results below for factual, time-sensitive sub-fields such as arrival requirements, documents required at immigration, and official links; cite the source URLs when you include factual claims):\n\n")

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

// ListSelectors returns the distinct destination + visa category
// combinations available to the public selector endpoint.
func (s *Service) ListSelectors(ctx context.Context, tenantID pgtype.UUID) ([]SelectorDTO, error) {
	rows, err := s.store.ListSelectors(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("arrivalchecklist: list selectors: %w", err)
	}

	dtos := make([]SelectorDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, selectorRowToDTO(r))
	}

	return dtos, nil
}

// GetGuide returns a single published guide by id.
func (s *Service) GetGuide(ctx context.Context, id, tenantID pgtype.UUID) (GuideDTO, error) {
	row, err := s.store.GetGuideByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GuideDTO{}, pgx.ErrNoRows
		}

		return GuideDTO{}, fmt.Errorf("arrivalchecklist: get guide: %w", err)
	}

	return guideRowToDTO(row), nil
}

// GetGuideItems returns the ordered checklist items for a published guide.
// The guide must exist, be published, active, and belong to the tenant.
// Public endpoint — never exposes draft/rejected guides (§92, ADR-088).
func (s *Service) GetGuideItems(ctx context.Context, guideID, tenantID pgtype.UUID) ([]ChecklistItemDTO, error) {
	if _, err := s.store.GetPublishedGuideByID(ctx, guideID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}

		return nil, fmt.Errorf("arrivalchecklist: get guide items: lookup guide: %w", err)
	}

	rows, err := s.store.GetGuideItems(ctx, guideID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("arrivalchecklist: get guide items: %w", err)
	}

	dtos := make([]ChecklistItemDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, itemRowToDTO(r))
	}

	return dtos, nil
}

// AdminList returns guides (any status) for the admin management view.
// The admin HITL queue must see draft/ai_draft/pending/rejected rows, so
// PublicOnly is false (no status filter unless one is explicitly passed).
func (s *Service) AdminList(ctx context.Context, tenantID, destSpaceID pgtype.UUID, visaCategory, applicantType, status, q, sort, order string, page, limit int32) ([]GuideDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.ListGuides(ctx, ListGuidesFilter{

		DestinationSpaceID: destSpaceID,
		VisaCategory:       visaCategory,
		ApplicantType:      applicantType,
		Status:             status,
		PublicOnly:         false,
		Q:                  q,
		Sort:               sort,
		Order:              order,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("arrivalchecklist: admin list: %w", err)
	}

	dtos := make([]GuideDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, guideRowToDTO(r))
	}

	return dtos, total, nil
}

// CreateGuide creates a new guide. Auth and permission checks are done
// at the handler layer.
func (s *Service) CreateGuide(ctx context.Context, tenantID pgtype.UUID, req CreateGuideRequest) (GuideDTO, error) {
	if req.DestinationSpaceID == "" {
		return GuideDTO{}, errors.New("destination_space_id is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(req.DestinationSpaceID); err != nil {
		return GuideDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var visaCategory *string

	if req.VisaCategory != "" {
		v := req.VisaCategory
		visaCategory = &v
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	row, err := s.store.CreateGuide(ctx, CreateGuideInput{
		DestinationSpaceID: destSpaceID,
		VisaCategory:       visaCategory,
		ApplicantType:      req.ApplicantType,
		Version:            req.Version,
		Status:             req.Status,
		IsActive:           isActive,
	})
	if err != nil {
		return GuideDTO{}, fmt.Errorf("arrivalchecklist: create guide: %w", err)
	}

	dto, err := s.getGuideDTOByID(ctx, row.ID, tenantID)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("arrivalchecklist: create guide: reload: %w", err)
	}

	return dto, nil
}

// UpdateGuide updates mutable fields on a guide. Only non-nil fields are
// applied. Auth and permission checks are done at the handler layer.
// The status field may only be set to 'draft', 'pending', or 'rejected' via
// this endpoint — publishing MUST go through ApproveGuide to enforce the
// ADR-088 HITL review gate (hitl_reviewed_by + hitl_reviewed_at).
func (s *Service) UpdateGuide(ctx context.Context, id, tenantID pgtype.UUID, req UpdateGuideRequest) (GuideDTO, error) {
	// Block status='published' via UpdateGuide — publishing must go through
	// ApproveGuide which records the HITL reviewer and timestamp (ADR-088).
	if req.Status != nil && *req.Status == StatusPublished {
		return GuideDTO{}, ErrPublishViaUpdate
	}

	in := UpdateGuideInput{
		IsActive:         req.IsActive,
		IsJapabotIndexed: req.IsJapabotIndexed,
		HITLReviewNotes:  req.HITLReviewNotes,
		Status:           req.Status,
		ApplicantType:    req.ApplicantType,
		Version:          req.Version,
		VisaCategory:     req.VisaCategory,
	}

	if req.DestinationSpaceID != nil && *req.DestinationSpaceID != "" {
		var sid pgtype.UUID
		if err := sid.Scan(*req.DestinationSpaceID); err != nil {
			return GuideDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
		}

		in.DestinationSpaceID = &sid
	}

	if err := s.store.UpdateGuide(ctx, id, tenantID, in); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GuideDTO{}, pgx.ErrNoRows
		}

		return GuideDTO{}, fmt.Errorf("arrivalchecklist: update guide: %w", err)
	}

	dto, err := s.getGuideDTOByID(ctx, id, tenantID)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("arrivalchecklist: update guide: reload: %w", err)
	}

	return dto, nil
}

// DeleteGuide deletes an arrival checklist guide and its items by id.
// Auth and permission checks are done at the handler/agent layer.
func (s *Service) DeleteGuide(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.DeleteGuide(ctx, id, tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("arrivalchecklist: delete guide: %w", err)
	}

	return nil
}

// ApproveGuide performs the human-in-the-loop approval: sets
// hitl_reviewed_by, hitl_reviewed_at = now(), and status = 'published'.
func (s *Service) ApproveGuide(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, req ApproveGuideRequest) (GuideDTO, error) {
	if err := s.store.ApproveGuide(ctx, id, tenantID, reviewerID, req.Notes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GuideDTO{}, pgx.ErrNoRows
		}

		return GuideDTO{}, fmt.Errorf("arrivalchecklist: approve guide: %w", err)
	}

	dto, err := s.getGuideDTOByID(ctx, id, tenantID)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("arrivalchecklist: approve guide: reload: %w", err)
	}

	return dto, nil
}

// GenerateGuide creates an AI-drafted arrival checklist guide in 'ai_draft'
// status. The draft is marked ai_generated=true with ai_model='content_agent_v1'
// and requires HITL approval (ApproveGuide) before publication. Admin-only
// (arrival_checklist.manage permission checked at route level).
// // When an AI dispatcher is wired (SetDispatcher), the method calls the AI
// Gateway via the "content_generation" category to draft checklist content
// from the destination, visa category, applicant type, and optional custom
// prompt. The SHA-256 of the prompt is stored as ai_prompt_hash for audit.
// If the dispatcher is nil or the call fails, the draft is still created so
// a reviewer can fill it manually (graceful degradation).
// Ref: ADR-088, geo-aware-tool-pattern.md.
func (s *Service) GenerateGuide(ctx context.Context, tenantID pgtype.UUID, req GenerateGuideRequest) (GenerateGuideResponse, error) {
	if req.DestinationSpaceID == "" {
		return GenerateGuideResponse{}, errors.New("destination_space_id is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(req.DestinationSpaceID); err != nil {
		return GenerateGuideResponse{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	applicantType := req.ApplicantType
	if applicantType == "" {
		applicantType = ApplicantIndividual
	}

	var visaCategory *string

	if req.VisaCategory != "" {
		v := req.VisaCategory
		visaCategory = &v
	}

	// Look up the destination name so the prompt and research queries are
	// human-readable. A missing name is non-fatal; we fall back to the ID.
	destName, _ := s.store.GetSpaceName(ctx, destSpaceID, tenantID)

	location := destName
	if location == "" {
		location = req.DestinationSpaceID
	}

	// Build the prompt sent to the Content Agent. The destination, visa
	// category, applicant type, and an optional caller-supplied prompt are
	// combined into a single string the gateway can route to the model.
	prompt := req.Prompt
	if prompt == "" {
		prompt = fmt.Sprintf(
			"Generate an arrival checklist guide for destination %s (destination_space_id=%s), visa_category=%s, applicant_type=%s. "+
				"Include ordered checklist items grouped by arrival phase (day_1_7, week_2_4, month_1_3, ongoing), required documents, immigration steps, and official links.\n\n"+
				"Use the GROUNDED RESEARCH section below for current arrival requirements and documents. Cite the source URLs when you make factual claims. If no grounded research is available, use your training knowledge and clearly note that details may need verification.",
			location, req.DestinationSpaceID, req.VisaCategory, applicantType,
		)
	} else {
		prompt = fmt.Sprintf(
			"%s\n\nContext: destination=%s, destination_space_id=%s, visa_category=%s, applicant_type=%s. "+
				"Use the GROUNDED RESEARCH section below for current arrival requirements and documents. Cite the source URLs when you make factual claims.",
			prompt, location, req.DestinationSpaceID, req.VisaCategory, applicantType,
		)
	}

	// Augment the prompt with grounded web research. If research fails, log
	// a warning and continue with the original prompt (graceful degradation).
	var citySpaceID pgtype.UUID // not used yet; reserved for future refinement
	if s.researcher != nil {
		researchText, _, researchErr := s.researcher.Research(ctx, tenantID, destSpaceID, citySpaceID, req.VisaCategory)
		if researchErr != nil {
			slog.Warn("arrivalchecklist: web research failed, continuing with original prompt", "error", researchErr)
		} else if researchText != "" {
			prompt = prompt + "\n\n" + researchText
		}
	}

	// ai_prompt_hash = SHA256(prompt) for audit traceability.
	sum := sha256.Sum256([]byte(prompt))
	promptHash := hex.EncodeToString(sum[:])

	// Call the AI Gateway (content_generation category) when a dispatcher
	// is wired. If the dispatcher is nil (tests), the draft is created
	// with no items. If the dispatcher is wired and the call fails, the
	// generation fails hard — the admin should retry rather than receive
	// an empty draft.
	var checklistItems []CreateItemInput

	if s.dispatcher != nil {
		resp, aiErr := s.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
			Category: "content_generation",
			Payload: map[string]any{
				"prompt":     prompt,
				"tenant_id":  tenantID,
				"guide_type": "arrival_checklist",
			},
		})
		if aiErr != nil {
			return GenerateGuideResponse{}, fmt.Errorf("arrivalchecklist: AI generation failed: %w", aiErr)
		}

		checklistItems = parseChecklistItemsFromAIResponse(resp)
		slog.Info("arrivalchecklist: AI Gateway generated draft items", "destination_space_id", req.DestinationSpaceID, "visa_category", req.VisaCategory, "applicant_type", applicantType, "prompt_hash", promptHash, "item_count", len(checklistItems), )
	}

	aiModel := "content_agent_v1"

	row, err := s.store.CreateGuide(ctx, CreateGuideInput{

		DestinationSpaceID: destSpaceID,
		VisaCategory:       visaCategory,
		ApplicantType:      applicantType,
		Version:            1,
		Status:             StatusAIDraft,
		IsActive:           false,
		AIGenerated:        true,
		AIModel:            &aiModel,
		AIPromptHash:       &promptHash,
	})
	if err != nil {
		return GenerateGuideResponse{}, fmt.Errorf("arrivalchecklist: generate draft: %w", err)
	}

	// Create the AI-generated checklist items for the new guide.
	if len(checklistItems) > 0 {
		for i := range checklistItems {
			checklistItems[i].GuideID = row.ID
		}

		if err := s.store.CreateItems(ctx, checklistItems); err != nil {
			slog.Warn("arrivalchecklist: create items failed, guide draft remains", "guide_id", row.ID, "error", err)
		}
	}

	return GenerateGuideResponse{
		Status:             "accepted",
		Message:            "Content Agent draft created in 'ai_draft' status. Review and approve to publish.",
		GuideID:            row.ID.String(),
		DestinationSpaceID: destSpaceID.String(),
		VisaCategory:       req.VisaCategory,
		ApplicantType:      applicantType,
	}, nil
}

// parseChecklistItemsFromAIResponse extracts checklist items from the AI
// gateway response. The response may contain a structured "items" array or
// plain text; plain text is wrapped into a single item per line.
func parseChecklistItemsFromAIResponse(resp ai.Response) []CreateItemInput {
	// Structured: {"items": [{"phase": "day_1_7", "title": "...", "description": "..."}, ...]}
	if itemsRaw, ok := resp.Raw["items"]; ok {
		if b, err := json.Marshal(itemsRaw); err == nil {
			var parsed []struct {
				Phase        string `json:"phase"`
				Title        string `json:"title"`
				Description  string `json:"description"`
				OfficialLink string `json:"official_link"`
			}

			if json.Unmarshal(b, &parsed) == nil {
				result := make([]CreateItemInput, 0, len(parsed))

				for i, p := range parsed {
					if p.Title == "" || p.Phase == "" {
						continue
					}

					item := CreateItemInput{
						Phase:     p.Phase,
						Title:     p.Title,
						SortOrder: int32(i), // #nosec
					}
					if p.Description != "" {
						item.Description = &p.Description
					}

					if p.OfficialLink != "" {
						item.OfficialLink = &p.OfficialLink
					}

					result = append(result, item)
				}

				return result
			}
		}
	}
	// Fallback: plain text under text/content/output — split into items.
	var text string
	if t, ok := resp.Raw["text"].(string); ok {
		text = t
	} else if t, ok := resp.Raw["content"].(string); ok {
		text = t
	} else if t, ok := resp.Raw["output"].(string); ok {
		text = t
	}

	if text == "" {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(text), "\n")
	result := make([]CreateItemInput, 0, len(lines))

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip common list markers (-, *, 1.).
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")

		if idx := strings.Index(line, ". "); idx >= 0 && idx < 4 {
			line = line[idx+2:]
		}

		phase := "ongoing"
		if i < 5 {
			phase = "day_1_7"
		} else if i < 12 {
			phase = "week_2_4"
		} else if i < 20 {
			phase = "month_1_3"
		}

		result = append(result, CreateItemInput{
			Phase:     phase,
			Title:     line,
			SortOrder: int32(i), // #nosec
		})
	}

	return result
}

// UserChecklistDTO is the API response for a user's saved arrival checklist
// (GET /me/arrival-checklists). Includes progress summary counts.
type UserChecklistDTO struct {
	GuideID         string `json:"guide_id"`
	DestSpaceName   string `json:"destination_space_name,omitempty"`
	DestSpaceCode   string `json:"destination_space_code,omitempty"`
	VisaCategory    string `json:"visa_category,omitempty"`
	ApplicantType   string `json:"applicant_type"`
	TotalItems      int64  `json:"total_items"`
	CompletedItems  int64  `json:"completed_items"`
	FirstProgressAt string `json:"first_progress_at,omitempty"`
	LastProgressAt  string `json:"last_progress_at,omitempty"`
}

// UserChecklistItemDTO is a single item in a user's saved arrival checklist
// with progress state.
type UserChecklistItemDTO struct {
	ID           string `json:"id"`
	GuideID      string `json:"guide_id"`
	Phase        string `json:"phase"`
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	OfficialLink string `json:"official_link,omitempty"`
	SortOrder    int32  `json:"sort_order"`
	IsFlagged    bool   `json:"is_flagged"`
	Completed    bool   `json:"completed"`
	CompletedAt  string `json:"completed_at,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

// StartChecklistRequest is the body for POST /me/arrival-checklists.
type StartChecklistRequest struct {
	GuideID string `json:"guide_id"`
}

// TickItemRequest is the body for PATCH /me/arrival-checklists/:id/items/:item_id.
type TickItemRequest struct {
	Completed *bool   `json:"completed"`
	Notes     *string `json:"notes"`
}

// ListUserChecklists returns the user's saved arrival checklists with progress
// summaries. For GET /me/arrival-checklists.
func (s *Service) ListUserChecklists(ctx context.Context, tenantID, userID pgtype.UUID) ([]UserChecklistDTO, error) {
	rows, err := s.store.ListUserChecklists(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("arrivalchecklist: list user checklists: %w", err)
	}

	dtos := make([]UserChecklistDTO, 0, len(rows))

	for _, r := range rows {
		dto := UserChecklistDTO{
			GuideID:        r.GuideID.String(),
			ApplicantType:  r.ApplicantType,
			TotalItems:     r.TotalItems,
			CompletedItems: r.CompletedItems,
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

		dto.FirstProgressAt = formatTs(r.FirstProgressAt)
		dto.LastProgressAt = formatTs(r.LastProgressAt)
		dtos = append(dtos, dto)
	}

	return dtos, nil
}

// GetUserChecklistItems returns the items for a single saved checklist with
// the user's progress state merged in. For GET /me/arrival-checklists/:id.
// Returns pgx.ErrNoRows if the guide does not exist or is not published.
func (s *Service) GetUserChecklistItems(ctx context.Context, tenantID, userID, guideID pgtype.UUID) ([]UserChecklistItemDTO, error) {
	// Verify the guide exists and is published.
	if _, err := s.store.GetPublishedGuideByID(ctx, guideID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}

		return nil, fmt.Errorf("arrivalchecklist: get user checklist items: lookup guide: %w", err)
	}

	rows, err := s.store.GetUserChecklistItems(ctx, tenantID, userID, guideID)
	if err != nil {
		return nil, fmt.Errorf("arrivalchecklist: get user checklist items: %w", err)
	}

	dtos := make([]UserChecklistItemDTO, 0, len(rows))

	for _, r := range rows {
		dto := UserChecklistItemDTO{
			ID:        r.ID.String(),
			GuideID:   r.GuideID.String(),
			Phase:     r.Phase,
			Title:     r.Title,
			SortOrder: r.SortOrder,
			IsFlagged: r.IsFlagged,
			Completed: r.Completed,
		}
		if r.Description != nil {
			dto.Description = *r.Description
		}

		if r.OfficialLink != nil {
			dto.OfficialLink = *r.OfficialLink
		}

		dto.CompletedAt = formatTs(r.CompletedAt)
		if r.Notes != nil {
			dto.Notes = *r.Notes
		}

		dtos = append(dtos, dto)
	}

	return dtos, nil
}

// StartChecklist starts a checklist for the user (creates an initial progress
// row). For POST /me/arrival-checklists. Returns the guide DTO so the frontend
// can render items immediately.
func (s *Service) StartChecklist(ctx context.Context, tenantID, userID pgtype.UUID, req StartChecklistRequest) (GuideDTO, error) {
	if req.GuideID == "" {
		return GuideDTO{}, errors.New("guide_id is required")
	}

	var guideID pgtype.UUID
	if err := guideID.Scan(req.GuideID); err != nil {
		return GuideDTO{}, fmt.Errorf("invalid guide_id: %w", err)
	}

	// Verify the guide exists and is published (§92 — only published guides
	// can be started by users; draft/ai_draft/pending/rejected are not
	// visible to non-admin callers per ADR-088 HITL gate).
	guide, err := s.store.GetPublishedGuideByID(ctx, guideID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GuideDTO{}, pgx.ErrNoRows
		}

		return GuideDTO{}, fmt.Errorf("arrivalchecklist: start checklist: lookup guide: %w", err)
	}

	if err := s.store.StartChecklist(ctx, tenantID, userID, guideID); err != nil {
		return GuideDTO{}, fmt.Errorf("arrivalchecklist: start checklist: %w", err)
	}

	return guideRowToDTO(guide), nil
}

// TickItem marks a single arrival checklist item complete/incomplete with an
// optional note. For PATCH /me/arrival-checklists/:id/items/:item_id.
// Only published+active guides accept progress updates (§92, ADR-088).
func (s *Service) TickItem(ctx context.Context, tenantID, userID, guideID pgtype.UUID, itemID string, req TickItemRequest) error {
	// Verify the guide exists and is published.
	if _, err := s.store.GetPublishedGuideByID(ctx, guideID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("arrivalchecklist: tick item: lookup guide: %w", err)
	}

	completed := false
	if req.Completed != nil {
		completed = *req.Completed
	}

	if err := s.store.TickItem(ctx, tenantID, userID, guideID, itemID, completed, req.Notes); err != nil {
		return fmt.Errorf("arrivalchecklist: tick item: %w", err)
	}

	return nil
}

// getGuideDTOByID fetches a guide by id and converts to DTO.
func (s *Service) getGuideDTOByID(ctx context.Context, id, tenantID pgtype.UUID) (GuideDTO, error) {
	row, err := s.store.GetGuideByID(ctx, id)
	if err != nil {
		return GuideDTO{}, err
	}

	return guideRowToDTO(row), nil
}

// guideRowToDTO converts a GuideRow to a GuideDTO.
func guideRowToDTO(r GuideRow) GuideDTO {
	dto := GuideDTO{
		ID:               r.ID.String(),
		ApplicantType:    r.ApplicantType,
		Version:          r.Version,
		Status:           r.Status,
		IsActive:         r.IsActive,
		AIGenerated:      r.AIGenerated,
		IsJapabotIndexed: r.IsJapabotIndexed,
		CreatedAt:        formatTs(r.CreatedAt),
		UpdatedAt:        formatTs(r.UpdatedAt),
		HITLReviewedAt:   formatTs(r.HITLReviewedAt),
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

	if r.AIModel != nil {
		dto.AIModel = *r.AIModel
	}

	if r.AIPromptHash != nil {
		dto.AIPromptHash = *r.AIPromptHash
	}

	if r.HITLReviewedBy.Valid {
		dto.HITLReviewedBy = r.HITLReviewedBy.String()
	}

	if r.HITLReviewedByName != nil {
		dto.HITLReviewedByName = *r.HITLReviewedByName
	}

	if r.HITLReviewNotes != nil {
		dto.HITLReviewNotes = *r.HITLReviewNotes
	}

	return dto
}

// itemRowToDTO converts an ItemRow to a ChecklistItemDTO.
func itemRowToDTO(r ItemRow) ChecklistItemDTO {
	dto := ChecklistItemDTO{
		ID:        r.ID.String(),
		GuideID:   r.GuideID.String(),
		Phase:     r.Phase,
		Title:     r.Title,
		SortOrder: r.SortOrder,
		IsFlagged: r.IsFlagged,
		CreatedAt: formatTs(r.CreatedAt),
	}
	if r.Description != nil {
		dto.Description = *r.Description
	}

	if r.OfficialLink != nil {
		dto.OfficialLink = *r.OfficialLink
	}

	dto.OriginCountryModifier = parseJSONB(r.OriginCountryModifier)

	return dto
}

// selectorRowToDTO converts a SelectorRow to a SelectorDTO.
func selectorRowToDTO(r SelectorRow) SelectorDTO {
	dto := SelectorDTO{
		GuideCount: r.GuideCount,
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

	if r.GuideID.Valid {
		dto.GuideID = r.GuideID.String()
	}

	return dto
}

// parseJSONB decodes a JSONB []byte into a generic value for the DTO.
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
