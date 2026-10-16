package publicapi

// Phase 3 (spec 2026-09-24-agent-data-api-phase3): POST /public/content —
// API-key draft creation. Everything the key writes lands as a draft;
// publish stays session-side. Requires the admin-granted api.write.content
// scope and the Idempotency-Key header (wired at the route).
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// publicDraftContentTypes is the Phase 3 allowlist for POST /public/content.
// Video, service listings, and other specialized types stay session-side —
// they carry required extension data a generic draft body can't supply.
var publicDraftContentTypes = map[string]bool{
	"blog_post":  true,
	"guide":      true,
	"story":      true,
	"discussion": true,
}

// CreateDraftInput is the accepted body for POST /public/content.
type CreateDraftInput struct {
	ContentType   string          `json:"content_type"`
	Title         string          `json:"title"`
	Slug          string          `json:"slug,omitempty"`
	Body          string          `json:"body"`
	BodyHTML      string          `json:"body_html,omitempty"`
	SpaceID       string          `json:"space_id,omitempty"`
	Tags          []string        `json:"tags,omitempty"`
	FieldValues   json.RawMessage `json:"field_values,omitempty"`
	JourneyStage  *string         `json:"journey_stage,omitempty"`
	VisaRoute     *string         `json:"visa_route,omitempty"`
	SourceCountry *string         `json:"source_country,omitempty"`
	TargetCountry *string         `json:"target_country,omitempty"`
	// Status is accepted only so we can reject it explicitly — any value
	// other than "draft" is a 400 (public keys never publish directly).
	Status string `json:"status,omitempty"`
}

// PublicDraft is the wire shape returned after draft creation.
type PublicDraft struct {
	ID          string `json:"id"`
	ContentType string `json:"content_type"`
	Title       string `json:"title"`
	Slug        string `json:"slug"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

// ContentDraftService is the narrow seam into the content domain. Satisfied
// by an adapter in internal/server over *content.Service + *tags.Service.
type ContentDraftService interface {
	CreateDraft(ctx context.Context, ownerID pgtype.UUID, in CreateDraftInput) (PublicDraft, error)
}

// SetContentDraftService injects the draft-creation adapter.
func (h *Handlers) SetContentDraftService(svc ContentDraftService) {
	h.drafts = svc
}

// CreatePublicDraft handles POST /api/v1/public/content.
// Requires api.write.content. The route is wrapped in
// mw.IdempotencyMiddleware — a missing Idempotency-Key is a 400 before the
// handler runs.
func (h *Handlers) CreatePublicDraft(w http.ResponseWriter, r *http.Request) {
	keyCtx := APIKeyFromContext(r.Context())
	if keyCtx == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
		return
	}

	if h.drafts == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_ready", "Public API not configured")
		return
	}

	// The middleware dedupes when a key is present but tolerates absence —
	// this endpoint requires the header outright (spec: retries must never
	// double-create).
	if r.Header.Get("Idempotency-Key") == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key header is required")
		return
	}

	var input CreateDraftInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	if input.ContentType == "" {
		input.ContentType = "discussion"
	}

	if !publicDraftContentTypes[input.ContentType] {
		errpkg.WriteError(w, http.StatusBadRequest, "unsupported_content_type",
			"content_type must be one of: blog_post, guide, story, discussion")
		return
	}
	// Publish is a session-side act — a caller-supplied status is rejected
	// outright (never silently clamped) so integrations can't assume they
	// published when they didn't.
	if input.Status != "" && input.Status != "draft" {
		errpkg.WriteError(w, http.StatusBadRequest, "status_not_allowed",
			"public API keys can only create drafts — omit status or set \"draft\"")
		return
	}

	draft, err := h.drafts.CreateDraft(r.Context(), keyCtx.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": draft})
}
