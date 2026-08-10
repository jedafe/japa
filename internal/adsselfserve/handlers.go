// Package adsselfserve implements the Self-Serve Advertising surface
// (Section 61, ADR-079).
//
// Ref: japaconnect-engineering/sections/61-self-serve-ads.md
//      japaconnect-engineering/adr/ADR-079-self-serve-ads.md
package adsselfserve

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the self-serve ads HTTP routes.
type Handlers struct {
	svc *Service
}

// NewHandlers creates self-serve ads handlers.
func NewHandlers(svc *Service) *Handlers { return &Handlers{svc: svc} }

// HandleListSlots returns available ad slot types with flat_rate.
// GET /org/{org_id}/ads/slots
func (h *Handlers) HandleListSlots(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}

	slots, err := h.svc.ListSlots(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": slots})
}

// HandleCreateCampaign creates a self-serve ad campaign.
// POST /org/{org_id}/ads/campaigns
func (h *Handlers) HandleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}

	var input CreateCampaignInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}
	if input.Name == "" || input.SlotTypeID == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "name and slot_type_id are required")
		return
	}

	campaign, err := h.svc.CreateCampaign(r.Context(), tenantID, orgID, userID, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrFeatureDisabled):
			// ADR-042: disabled features return 404, not 403.
			writeError(w, http.StatusNotFound, "ads_self_serve_disabled", "Self-serve advertising is currently disabled")
		case errors.Is(err, ErrOrganizationNotVerified):
			writeError(w, http.StatusForbidden, "organization_not_verified", "Organization must be verified to create ads")
		case errors.Is(err, ErrNotOrgMember):
			writeError(w, http.StatusForbidden, "not_org_member", "You are not a member of this organization")
		case errors.Is(err, ErrSlotTypeNotFound):
			writeError(w, http.StatusBadRequest, "slot_type_not_found", "The selected ad slot type was not found or has no rate")
		case errors.Is(err, ErrDurationTooShort):
			writeError(w, http.StatusBadRequest, "duration_too_short", err.Error())
		default:
			writeError(w, http.StatusBadRequest, "create_failed", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"data": map[string]interface{}{
			"id":             campaign.ID.String(),
			"name":           campaign.Name,
			"status":         campaign.Status,
			"budget_credits": campaign.BudgetCredits,
			"flat_rate":      campaign.FlatRate,
		},
	})
}

// HandleListCampaigns lists an org's campaigns.
// GET /org/{org_id}/ads/campaigns
func (h *Handlers) HandleListCampaigns(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	page, limit := parsePagination(r)

	campaigns, total, err := h.svc.ListCampaigns(r.Context(), tenantID, orgID, status, page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  campaigns,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// HandleGetCampaign returns a single campaign detail.
// GET /org/{org_id}/ads/campaigns/{id}
func (h *Handlers) HandleGetCampaign(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}
	campaignID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	campaign, err := h.svc.GetCampaign(r.Context(), tenantID, orgID, campaignID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Campaign not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": campaign})
}

// HandleUpdateCampaign updates a campaign's creative (pending/paused only).
// PATCH /org/{org_id}/ads/campaigns/{id}
func (h *Handlers) HandleUpdateCampaign(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}
	campaignID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Content   json.RawMessage `json:"content"`
		TargetURL string          `json:"target_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	campaign, err := h.svc.UpdateCampaignCreative(r.Context(), tenantID, orgID, campaignID, body.Content, body.TargetURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "update_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": campaign})
}

// HandlePauseCampaign pauses an active campaign.
// POST /org/{org_id}/ads/campaigns/{id}/pause
func (h *Handlers) HandlePauseCampaign(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}
	campaignID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	campaign, err := h.svc.PauseCampaign(r.Context(), tenantID, orgID, campaignID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "pause_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"id":     campaign.ID.String(),
			"status": campaign.Status,
		},
	})
}

// HandleResumeCampaign resumes a paused campaign.
// POST /org/{org_id}/ads/campaigns/{id}/resume
func (h *Handlers) HandleResumeCampaign(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}
	campaignID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	campaign, err := h.svc.ResumeCampaign(r.Context(), tenantID, orgID, campaignID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "resume_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"id":     campaign.ID.String(),
			"status": campaign.Status,
		},
	})
}

// HandleGetStats returns campaign statistics.
// GET /org/{org_id}/ads/campaigns/{id}/stats
func (h *Handlers) HandleGetStats(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}
	campaignID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	stats, err := h.svc.GetCampaignStats(r.Context(), tenantID, orgID, campaignID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Campaign not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": stats})
}

// --- shared helpers (same pattern as internal/story/handlers.go) ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
			"status":  status,
		},
	})
}

// requireTenant extracts the tenant ID from the request context. The tenant
// is set by tenant.Middleware which wraps all /api/v1 routes.
func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

// requireUser extracts the authenticated user from the request context. The
// user is set by auth.RequireAuth middleware which wraps all ads routes.
func requireUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return pgtype.UUID{}, false
	}
	return u.UserID, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, param string) (pgtype.UUID, bool) {
	raw := chi.URLParam(r, param)
	if raw == "" {
		writeError(w, http.StatusBadRequest, "missing_param", param+" is required")
		return pgtype.UUID{}, false
	}
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_uuid", "Invalid "+param)
		return pgtype.UUID{}, false
	}
	return id, true
}

func parsePagination(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return page, limit
}
