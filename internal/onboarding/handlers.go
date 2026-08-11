package onboarding

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for onboarding HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
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

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	status, err := h.Service.GetState(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": status})
}

// UpdateStep handles PUT /api/v1/onboarding/step.
func (h *Handlers) UpdateStep(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req UpdateStepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdateStep(r.Context(), tenantID, user.UserID, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// Complete handles POST /api/v1/onboarding/complete.
func (h *Handlers) Complete(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.Complete(r.Context(), tenantID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "completed"}})
}

// GetRecommendations handles GET /api/v1/onboarding/recommendations.
func (h *Handlers) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	recs, err := h.Service.GetRecommendations(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": recs})
}

// DismissTour handles POST /api/v1/onboarding/tour/dismiss.
func (h *Handlers) DismissTour(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.DismissTour(r.Context(), tenantID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "dismissed"}})
}

// GetOptions handles GET /api/v1/onboarding/options.
// Returns the option lists for Step 1 (japa status) and Step 2 (primary goal)
// so the frontend does not hardcode them (Rule 16). The option values are
// the canonical machine-readable keys stored in step_data; the frontend
// resolves the label_key via i18n for display.
func (h *Handlers) GetOptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
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

// GetAdminSettings handles GET /admin/onboarding/settings.
func (h *Handlers) GetAdminSettings(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	settings, err := h.Service.GetAdminSettings(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": settings})
}

// UpdateAdminSettings handles PUT /admin/onboarding/settings.
func (h *Handlers) UpdateAdminSettings(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req UpdateOnboardingSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdateAdminSettings(r.Context(), tenantID, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// ListTemplates handles GET /admin/onboarding/templates.
func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	templates, err := h.Service.ListTemplates(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": templates})
}

// CreateTemplate handles POST /admin/onboarding/templates.
func (h *Handlers) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req CreateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}

	template, err := h.Service.CreateTemplate(r.Context(), tenantID, &req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": template})
}

// UpdateTemplate handles PATCH /admin/onboarding/templates/:id.
func (h *Handlers) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}

	var req UpdateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdateTemplate(r.Context(), tenantID, id, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// DeleteTemplate handles DELETE /admin/onboarding/templates/:id.
func (h *Handlers) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid template ID")
		return
	}

	if err := h.Service.DeleteTemplate(r.Context(), tenantID, id); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}
