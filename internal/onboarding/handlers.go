package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
)

// Handlers holds dependencies for onboarding HTTP handlers.
type Handlers struct {
	Service *Service
	lit     literalResolver // nil disables literal resolution (§14)
}

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14). Implemented by *literals.Catalog; nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// SetLiteralResolver injects the template-literal catalog so user-facing
// titles/descriptions render {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

// resolveLiterals resolves {{...}} placeholders on a response DTO or slice.
// Best-effort — failures are logged inside the resolver.
func (h *Handlers) resolveLiterals(r *http.Request, v any) {
	if literals.IsRawRequest(r) {
		return
	}

	if h.lit == nil {
		return
	}

	h.lit.ResolveFields(r.Context(), v, literals.NewRequestContext(r, ""))
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate
// HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// ---------------------------------------------------------------------------
// Onboarding Handlers
// ---------------------------------------------------------------------------

// GetStatus handles GET /api/v1/onboarding/status.
func (h *Handlers) GetStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	status, err := h.Service.GetState(r.Context(), user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &status)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": status})
}

// UpdateStep handles PUT /api/v1/onboarding/step.
func (h *Handlers) UpdateStep(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req UpdateStepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdateStep(r.Context(), user.UserID, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// Complete handles POST /api/v1/onboarding/complete.
func (h *Handlers) Complete(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	if err := h.Service.Complete(r.Context(), user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "completed"}})
}

// GetRecommendations handles GET /api/v1/onboarding/recommendations.
func (h *Handlers) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	recs, err := h.Service.GetRecommendations(r.Context(), user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &recs)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": recs})
}

// DismissTour handles POST /api/v1/onboarding/tour/dismiss. The body is
// optional — {"surface": "docs"|"academy"|...} records a per-surface
// dismissal; an empty body defaults to the "main" surface (Section 09).
func (h *Handlers) DismissTour(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req DismissTourRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.DismissTour(r.Context(), user.UserID, req.Surface); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "dismissed"}})
}

// GetOptions handles GET /api/v1/onboarding/options.
// Returns the option lists for Step 1 (japa status) and Step 2 (primary goal)
// so the frontend does not hardcode them (Rule 16). The option values are
// the canonical machine-readable keys stored in step_data; the frontend
// resolves the label_key via i18n for display.
func (h *Handlers) GetOptions(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": OnboardingOptionsResponse{
			JapaStatuses: []OnboardingOption{
				{Value: "newly_japa", LabelKey: "onboarding.status_newly_japa"},
				{Value: "settling_in", LabelKey: "onboarding.status_settling_in"},
				{Value: "settled", LabelKey: "onboarding.status_settled"},
				{Value: "planning", LabelKey: "onboarding.status_planning"},
				{Value: "considering", LabelKey: "onboarding.status_considering"},
				{Value: "returned", LabelKey: "onboarding.status_returned"},
				{Value: "never_left", LabelKey: "onboarding.status_never_left"},
				{Value: "not_say", LabelKey: "onboarding.status_not_say"},
			},
			PrimaryGoals: []OnboardingOption{
				{Value: "work", LabelKey: "onboarding.goal_work"},
				{Value: "study", LabelKey: "onboarding.goal_study"},
				{Value: "business", LabelKey: "onboarding.goal_business"},
				{Value: "family", LabelKey: "onboarding.goal_family"},
				{Value: "lifestyle", LabelKey: "onboarding.goal_lifestyle"},
				{Value: "unsure", LabelKey: "onboarding.goal_unsure"},
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Admin Onboarding Handlers
// ---------------------------------------------------------------------------

// ListTemplates handles GET /admin/onboarding/templates.
func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.Service.ListTemplates(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &templates)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": templates})
}

// GetTemplate handles GET /admin/onboarding/templates/:id.
func (h *Handlers) GetTemplate(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}

	template, err := h.Service.GetTemplate(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": template})
}

// CreateTemplate handles POST /admin/onboarding/templates.
func (h *Handlers) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	var req CreateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}

	template, err := h.Service.CreateTemplate(r.Context(), &req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": template})
}

// UpdateTemplate handles PATCH /admin/onboarding/templates/:id.
func (h *Handlers) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}

	var req UpdateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdateTemplate(r.Context(), id, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// DeleteTemplate handles DELETE /admin/onboarding/templates/:id.
func (h *Handlers) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}

	if err := h.Service.DeleteTemplate(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// GetAdminSettings handles GET /admin/onboarding/settings.
// Returns the platform's onboarding configuration (minimum counts + promoted
// entities). Ref: Section 09 — admin Onboarding Settings surface.
func (h *Handlers) GetAdminSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.Service.GetAdminSettings(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": settings})
}

// UpdateAdminSettings handles PUT /admin/onboarding/settings.
// Applies a partial update to the onboarding configuration. Only fields
// present in the request body are written. Ref: Section 09.
func (h *Handlers) UpdateAdminSettings(w http.ResponseWriter, r *http.Request) {
	var req UpdateOnboardingSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if err := h.Service.UpdateAdminSettings(r.Context(), &req); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}
