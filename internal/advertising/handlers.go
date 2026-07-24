// Package advertising — HTTP handlers for the Ad Placement system.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
// V1 routes are admin-only.
package advertising

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/worker"
)

const defaultPageSize int32 = 20

// TaskEnqueueClient is the interface for enqueueing background tasks.
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Handlers holds dependencies for advertising HTTP handlers.
type Handlers struct {
	Service     *Service
	AsynqClient TaskEnqueueClient
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// enqueueAdEventIngest enqueues an async ad event ingestion task.
// Fire-and-forget: errors are logged but never propagated.
func (h *Handlers) enqueueAdEventIngest(tenantID, campaignID, userID pgtype.UUID, eventType string) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"tenant_id":   tenantID.String(),
		"campaign_id": campaignID.String(),
		"user_id":     userID.String(),
		"event_type":  eventType,
	})
	if err != nil {
		slog.Warn("ads: failed to marshal event ingest payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeAdEventIngest, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeAdEventIngest)),
	); err != nil {
		slog.Warn("ads: failed to enqueue event ingest task",
			"campaign_id", campaignID.String(), "error", err)
	}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrAlreadyExists), errors.Is(err, errpkg.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > int(MaxPageSize) {
		return MaxPageSize
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// parsePageSizeOffset extracts the page_offset query parameter.
func parsePageSizeOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// ---------------------------------------------------------------------------
// Campaign Handlers
// ---------------------------------------------------------------------------

// CreateCampaign handles POST /api/v1/admin/advertising/campaigns.
func (h *Handlers) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Name                string                 `json:"name"`
		SlotTypeName        string                 `json:"slot_type_name"`
		Content             map[string]interface{} `json:"content"`
		TargetURL           string                 `json:"target_url"`
		Budget              *int64                 `json:"budget"`
		Priority            int32                  `json:"priority"`
		FrequencyCapPerUser *int32                 `json:"frequency_cap_per_user"`
		StartDate           string                 `json:"start_date"`
		EndDate             string                 `json:"end_date"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	if input.SlotTypeName == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_slot_type_name", "slot_type_name is required")
		return
	}

	if input.Content == nil {
		httpx.WriteError(w, http.StatusBadRequest, "missing_content", "content is required")
		return
	}

	startDate := time.Now()

	if input.StartDate != "" {
		var err error

		startDate, err = time.Parse(time.RFC3339, input.StartDate)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_start_date", "Invalid start_date format")
			return
		}
	}

	var endDate *time.Time

	if input.EndDate != "" {
		t, err := time.Parse(time.RFC3339, input.EndDate)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_end_date", "Invalid end_date format")
			return
		}

		endDate = &t
	}

	campaign, err := h.Service.CreateCampaign(r.Context(), tenantID, user.UserID, CreateCampaignInput{
		Name:                input.Name,
		SlotTypeName:        input.SlotTypeName,
		Content:             input.Content,
		TargetURL:           input.TargetURL,
		Budget:              input.Budget,
		Priority:            input.Priority,
		FrequencyCapPerUser: input.FrequencyCapPerUser,
		StartDate:           startDate,
		EndDate:             endDate,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": campaign})
}

// GetCampaign handles GET /api/v1/admin/advertising/campaigns/{id}.
func (h *Handlers) GetCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	campaign, err := h.Service.GetCampaign(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// ListCampaigns handles GET /api/v1/admin/advertising/campaigns.
func (h *Handlers) ListCampaigns(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	campaigns, err := h.Service.ListCampaigns(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": campaigns})
}

// ActivateCampaign handles POST /api/v1/admin/advertising/campaigns/{id}/activate.
func (h *Handlers) ActivateCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	campaign, err := h.Service.ActivateCampaign(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// PauseCampaign handles POST /api/v1/admin/advertising/campaigns/{id}/pause.
func (h *Handlers) PauseCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	campaign, err := h.Service.PauseCampaign(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// EndCampaign handles POST /api/v1/admin/advertising/campaigns/{id}/end.
func (h *Handlers) EndCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	campaign, err := h.Service.EndCampaign(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// ---------------------------------------------------------------------------
// Ad Rendering Handler
// ---------------------------------------------------------------------------

// RenderSlot handles GET /api/v1/ads/render?slot={slot_name}.
// Returns eligible campaigns for the given slot. Empty result when no match
// — never a placeholder. Public (catalog: GET /ads/serve) — guests are served
// the untargeted/default eligible set; usergroup-based targeting is not yet
// implemented (no targeting column on ad_campaigns — deferred, see plan P5).
func (h *Handlers) RenderSlot(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slotName := r.URL.Query().Get("slot")
	if slotName == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_slot", "slot query parameter is required")
		return
	}

	user := auth.FromContext(r.Context())

	var userID pgtype.UUID

	if user != nil {
		userID = user.UserID
	}

	campaigns, err := h.Service.RenderSlot(r.Context(), tenantID, userID, slotName)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": campaigns})
}

// ---------------------------------------------------------------------------
// Ad Events Ingestion Handler
// ---------------------------------------------------------------------------

// IngestEvents handles POST /api/v1/ads/events.
// Accepts a batch of ad events for async ingestion. Public (catalog: POST
// /ads/events) — fire-and-forget impression/click ingestion from guests too.
func (h *Handlers) IngestEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Events []struct {
			CampaignID string `json:"campaign_id"`
			UserID     string `json:"user_id"`
			EventType  string `json:"event_type"`
			OccurredAt string `json:"occurred_at"`
		} `json:"events"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Events) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "empty_events", "events array is empty")
		return
	}

	events := make([]IngestAdEventInput, 0, len(input.Events))

	for _, e := range input.Events {
		if e.EventType != EventTypeImpression && e.EventType != EventTypeClick {
			continue
		}

		var occurredAt time.Time

		if e.OccurredAt != "" {
			var err error

			occurredAt, err = time.Parse(time.RFC3339, e.OccurredAt)
			if err != nil {
				continue
			}
		}

		events = append(events, IngestAdEventInput{
			CampaignID: e.CampaignID,
			UserID:     e.UserID,
			EventType:  e.EventType,
			OccurredAt: occurredAt,
		})
	}

	ingested, err := h.Service.IngestAdEvents(r.Context(), tenantID, events)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue async ad event batch ingestion for background processing — fire-and-forget.
	for _, e := range events {
		var campaignID, userID pgtype.UUID
		if err := campaignID.Scan(e.CampaignID); err != nil {
			continue
		}
		if e.UserID != "" {
			_ = userID.Scan(e.UserID)
		}
		h.enqueueAdEventIngest(tenantID, campaignID, userID, e.EventType)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"ingested": ingested,
			"total":    len(input.Events),
		},
	})
}

// ---------------------------------------------------------------------------
// Slot Type Handlers
// ---------------------------------------------------------------------------

// ListSlotTypes handles GET /api/v1/admin/advertising/slot-types.
func (h *Handlers) ListSlotTypes(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	slotTypes, err := h.Service.ListSlotTypes(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": slotTypes})
}

// CreateSlotType handles POST /api/v1/admin/advertising/slot-types.
func (h *Handlers) CreateSlotType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
		MaxWidth    int    `json:"max_width"`
		MaxHeight   int    `json:"max_height"`
		Format      string `json:"format"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	slotType, err := h.Service.CreateSlotType(r.Context(), tenantID, AdSlotTypeInput{
		Name:        input.Name,
		Slug:        input.Slug,
		Description: input.Description,
		MaxWidth:    input.MaxWidth,
		MaxHeight:   input.MaxHeight,
		Format:      input.Format,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": slotType})
}

// UpdateSlotType handles PUT /api/v1/admin/advertising/slot-types/{id}.
func (h *Handlers) UpdateSlotType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid slot type ID")
		return
	}

	var input struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
		MaxWidth    int    `json:"max_width"`
		MaxHeight   int    `json:"max_height"`
		Format      string `json:"format"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	slotType, err := h.Service.UpdateSlotType(r.Context(), id, tenantID, AdSlotTypeInput{
		Name:        input.Name,
		Slug:        input.Slug,
		Description: input.Description,
		MaxWidth:    input.MaxWidth,
		MaxHeight:   input.MaxHeight,
		Format:      input.Format,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": slotType})
}

// DeleteSlotType handles DELETE /api/v1/admin/advertising/slot-types/{id}.
func (h *Handlers) DeleteSlotType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid slot type ID")
		return
	}

	if err := h.Service.DeleteSlotType(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// DeleteCampaign handles DELETE /api/v1/admin/advertising/campaigns/{id}.
func (h *Handlers) DeleteCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	if err := h.Service.DeleteCampaign(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// GetCampaignStats handles GET /api/v1/admin/advertising/campaigns/{id}/stats.
func (h *Handlers) GetCampaignStats(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	stats, err := h.Service.GetCampaignStats(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": stats})
}

// ---------------------------------------------------------------------------
// Organization Boost Handlers
// ---------------------------------------------------------------------------

// CreateBoost handles POST /api/v1/admin/advertising/boosts.
func (h *Handlers) CreateBoost(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		OrganizationID string `json:"organization_id"`
		CampaignID     string `json:"campaign_id"`
		BoostType      string `json:"boost_type"`
		StartDate      string `json:"start_date"`
		EndDate        string `json:"end_date"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.OrganizationID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_organization_id", "organization_id is required")
		return
	}

	if input.CampaignID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_campaign_id", "campaign_id is required")
		return
	}

	var organizationID pgtype.UUID
	if err := organizationID.Scan(input.OrganizationID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_organization_id", "Invalid organization_id")
		return
	}

	var campaignID pgtype.UUID
	if err := campaignID.Scan(input.CampaignID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_campaign_id", "Invalid campaign_id")
		return
	}

	startDate := time.Now()

	if input.StartDate != "" {
		var err error

		startDate, err = time.Parse(time.RFC3339, input.StartDate)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_start_date", "Invalid start_date format")
			return
		}
	}

	var endDate *time.Time

	if input.EndDate != "" {
		t, err := time.Parse(time.RFC3339, input.EndDate)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_end_date", "Invalid end_date format")
			return
		}

		endDate = &t
	}

	boost, err := h.Service.CreateBoost(r.Context(), tenantID, CreateBoostInput{
		OrganizationID: organizationID,
		CampaignID:     campaignID,
		BoostType:      input.BoostType,
		StartDate:      startDate,
		EndDate:        endDate,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": boost})
}

// ListBoosts handles GET /api/v1/admin/advertising/boosts?organization_id={id}.
func (h *Handlers) ListBoosts(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	orgIDStr := r.URL.Query().Get("organization_id")
	if orgIDStr == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_organization_id", "organization_id query parameter is required")
		return
	}

	var organizationID pgtype.UUID
	if err := organizationID.Scan(orgIDStr); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_organization_id", "Invalid organization_id")
		return
	}

	boosts, err := h.Service.ListBoosts(r.Context(), tenantID, organizationID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": boosts})
}

// ListActiveBoosts handles GET /api/v1/admin/advertising/boosts/active.
func (h *Handlers) ListActiveBoosts(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	boosts, err := h.Service.ListActiveBoosts(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": boosts})
}

// EndBoost handles POST /api/v1/admin/advertising/boosts/{id}/end.
func (h *Handlers) EndBoost(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid boost ID")
		return
	}

	boost, err := h.Service.EndBoost(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": boost})
}

// UpdateBoost handles PATCH /api/v1/admin/ads/boosts/{id}.
func (h *Handlers) UpdateBoost(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid boost ID")
		return
	}

	var input struct {
		Status  *string `json:"status"`
		EndDate *string `json:"end_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var endDate *time.Time
	if input.EndDate != nil && *input.EndDate != "" {
		t, perr := time.Parse(time.RFC3339, *input.EndDate)
		if perr != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_end_date", "Invalid end_date format")
			return
		}
		endDate = &t
	}

	boost, err := h.Service.UpdateBoost(r.Context(), id, tenantID, input.Status, endDate)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": boost})
}

// DeleteBoost handles DELETE /api/v1/admin/ads/boosts/{id}.
func (h *Handlers) DeleteBoost(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid boost ID")
		return
	}

	if err := h.Service.DeleteBoost(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// ─── P5.6 Advertising Extras ─────────────────────────────────────────────

// UpdateCampaign handles PATCH /admin/ads/campaigns/:id.
func (h *Handlers) UpdateCampaign(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Budget      *int64 `json:"budget"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	_, err = h.Service.db.Writer.Exec(r.Context(),
		`UPDATE ad_campaigns SET name = COALESCE(NULLIF($3,''), name),
		 description = COALESCE(NULLIF($4,''), description),
		 budget = COALESCE($5, budget), updated_at = now()
		 WHERE id = $1 AND tenant_id = $2`, id, tenantID, input.Name, input.Description, input.Budget)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": id.String()}})
}

// GetSlotType handles GET /admin/ads/slot-types/:id.
func (h *Handlers) GetSlotType(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid slot type ID")
		return
	}
	var slotType struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
	}
	err = h.Service.db.Reader.QueryRow(r.Context(),
		`SELECT id, name, COALESCE(description,''), width, height
		 FROM ad_slot_types WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&slotType.ID, &slotType.Name, &slotType.Description, &slotType.Width, &slotType.Height)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Slot type not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": slotType})
}
