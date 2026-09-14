// Package campaign — handlers.go provides the admin HTTP API for content
// campaigns (Section 104).
//
// Routes:
//
//	POST /api/v1/admin/campaigns              — create and plan a campaign
//	GET  /api/v1/admin/campaigns              — list campaigns
//	GET  /api/v1/admin/campaigns/:id          — campaign detail
//	POST /api/v1/admin/campaigns/:id/run      — run a planned campaign
//	POST /api/v1/admin/campaigns/items/:id/retry — retry a failed item
//	POST /api/v1/admin/campaigns/:id/cancel   — cancel a campaign
package campaign

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the admin campaign HTTP endpoints.
type Handlers struct {
	service *Service
}

// NewHandlers creates campaign handlers bound to the provided service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}

	return id, true
}

func parsePagination(r *http.Request) (int32, int32) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if limit <= 0 {
		limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("page_offset"))
	if offset < 0 {
		offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	}

	if offset < 0 {
		offset = 0
	}

	return int32(limit), int32(offset) // #nosec
}

// HandleCreate handles POST /api/v1/admin/campaigns.
func (h *Handlers) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "campaign_unavailable", "Campaign service not configured")
		return
	}

	var body struct {
		Prompt                string   `json:"prompt"`
		Name                  string   `json:"name"`
		ContentType           string   `json:"content_type"`
		Count                 int      `json:"count"`
		PersonaIDs            []string `json:"persona_ids"`
		AutoProvisionPersonas *bool    `json:"auto_provision_personas"`
		AutoAssign            *bool    `json:"auto_assign"`
		TargetSpaceID         string   `json:"target_space_id"`
		TargetTags            []string `json:"target_tags"`
		RunImmediately        *bool    `json:"run_immediately"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if body.Prompt == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "prompt is required")
		return
	}

	if body.Count <= 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "count must be greater than 0")
		return
	}

	autoProvision := true
	if body.AutoProvisionPersonas != nil {
		autoProvision = *body.AutoProvisionPersonas
	}

	autoAssign := true
	if body.AutoAssign != nil {
		autoAssign = *body.AutoAssign
	}

	runImmediately := false
	if body.RunImmediately != nil {
		runImmediately = *body.RunImmediately
	}

	in := CreateCampaignInput{
		Prompt:                body.Prompt,
		Name:                  body.Name,
		ContentType:           body.ContentType,
		Count:                 body.Count,
		PersonaPool:           body.PersonaIDs,
		AutoProvisionPersonas: autoProvision,
		AutoAssign:            autoAssign,
		TargetSpaceID:         body.TargetSpaceID,
		TargetTags:            body.TargetTags,
		RunImmediately:        runImmediately,
	}

	campaign, _, err := h.service.CreateCampaign(r.Context(), user.UserID, in)
	if err != nil {
		slog.Error("campaign: create failed", "error", err)
		h.writeServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": campaign})
}

// HandleList handles GET /api/v1/admin/campaigns.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "campaign_unavailable", "Campaign service not configured")
		return
	}

	limit, offset := parsePagination(r)
	f := CampaignListFilter{
		Status: r.URL.Query().Get("status"),
		Limit:  limit,
		Offset: offset,
	}

	campaigns, total, err := h.service.ListCampaigns(r.Context(), f)
	if err != nil {
		slog.Error("campaign: list failed", "error", err)
		h.writeServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": campaigns,
		"meta": map[string]int64{"total": total, "page_size": int64(limit), "page_offset": int64(offset)},
	})
}

// HandleGet handles GET /api/v1/admin/campaigns/:id.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "campaign_unavailable", "Campaign service not configured")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	campaign, items, err := h.service.GetCampaign(r.Context(), id)
	if err != nil {
		slog.Error("campaign: get failed", "error", err, "id", id.String())
		h.writeServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"campaign": campaign,
			"items":    items,
		},
	})
}

// HandleRun handles POST /api/v1/admin/campaigns/:id/run.
func (h *Handlers) HandleRun(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "campaign_unavailable", "Campaign service not configured")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	batch, jobs, err := h.service.RunCampaign(r.Context(), user.UserID, id)
	if err != nil {
		slog.Error("campaign: run failed", "error", err, "id", id.String())
		h.writeServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"batch": batch,
			"jobs":  jobs,
		},
	})
}

// HandleRetry handles POST /api/v1/admin/campaigns/items/:id/retry.
func (h *Handlers) HandleRetry(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "campaign_unavailable", "Campaign service not configured")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.RetryItem(r.Context(), user.UserID, id); err != nil {
		slog.Error("campaign: retry failed", "error", err, "item_id", id.String())
		h.writeServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"status": "queued"})
}

// HandleCancel handles POST /api/v1/admin/campaigns/:id/cancel.
func (h *Handlers) HandleCancel(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "campaign_unavailable", "Campaign service not configured")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.CancelCampaign(r.Context(), user.UserID, id); err != nil {
		slog.Error("campaign: cancel failed", "error", err, "id", id.String())
		h.writeServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}

func (h *Handlers) writeServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrPersonaPoolTooSmall) {
		errpkg.WriteError(w, http.StatusBadRequest, "persona_pool_too_small", err.Error())
		return
	}

	errpkg.HandleServiceError(w, err)
}
