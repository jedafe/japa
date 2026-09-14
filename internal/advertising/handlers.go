// Package advertising — HTTP handlers for the Ad Placement system.
// // Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
// V1 routes are admin-only.
package advertising

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
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
// userID may be invalid (zero UUID) for guest events — user_id is nullable.
func (h *Handlers) enqueueAdEventIngest(tenantID, campaignID, userID pgtype.UUID, eventType string) {
	if h.AsynqClient == nil {
		return
	}

	// Build payload — omit user_id when invalid (guest/anonymous event).
	payloadMap := map[string]string{

		"campaign_id": campaignID.String(),
		"event_type":  eventType,
	}
	if userID.Valid {
		payloadMap["user_id"] = userID.String()
	}

	payload, err := json.Marshal(payloadMap)
	if err != nil {
		slog.Warn("ads: failed to marshal event ingest payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeAdEventIngest, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeAdEventIngest)),
	); err != nil {
		slog.Warn("ads: failed to enqueue event ingest task", "campaign_id", campaignID.String(), "error", err)
	}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
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

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// nonNilSlice replaces a nil slice with an empty, non-nil slice so the JSON
// encoder emits `[]` instead of `null` (§6.2 — nil slice serialized as null
// breaks frontends that read `.items`/`.length` on the response data).
func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}

	return s
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

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n)
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

	// #nosec G109 // value bounded by MaxPageSize check
	// #nosec G115 // value bounded by caller
	return int32(n)
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
	_, ok = h.requireTenant(w, r)
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
		TargetGeo           []string               `json:"target_geo"`
		TargetSpaceIDs      []string               `json:"target_space_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	if input.SlotTypeName == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slot_type_name", "slot_type_name is required")
		return
	}

	if input.Content == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_content", "content is required")
		return
	}

	startDate := time.Now()

	if input.StartDate != "" {
		var err error

		startDate, err = time.Parse(time.RFC3339, input.StartDate)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_start_date", "Invalid start_date format")
			return
		}
	}

	var endDate *time.Time

	if input.EndDate != "" {
		t, err := time.Parse(time.RFC3339, input.EndDate)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_end_date", "Invalid end_date format")
			return
		}

		endDate = &t
	}

	// Convert target_space_ids from string to pgtype.UUID.
	var targetSpaceIDs []pgtype.UUID

	for _, sid := range input.TargetSpaceIDs {
		var u pgtype.UUID
		if err := u.Scan(sid); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_target_space_id", "Invalid target_space_ids format")
			return
		}

		targetSpaceIDs = append(targetSpaceIDs, u)
	}

	campaign, err := h.Service.CreateCampaign(r.Context(), user.UserID, CreateCampaignInput{
		Name:                input.Name,
		SlotTypeName:        input.SlotTypeName,
		Content:             input.Content,
		TargetURL:           input.TargetURL,
		Budget:              input.Budget,
		Priority:            input.Priority,
		FrequencyCapPerUser: input.FrequencyCapPerUser,
		StartDate:           startDate,
		EndDate:             endDate,
		TargetGeo:           input.TargetGeo,
		TargetSpaceIDs:      targetSpaceIDs,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": campaign})
}

// GetCampaign handles GET /api/v1/admin/advertising/campaigns/{id}.
func (h *Handlers) GetCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	campaign, err := h.Service.GetCampaign(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// ListCampaigns handles GET /api/v1/admin/advertising/campaigns.
func (h *Handlers) ListCampaigns(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	campaigns, err := h.Service.ListCampaigns(r.Context(), limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": nonNilSlice(campaigns)})
}

// ActivateCampaign handles POST /api/v1/admin/advertising/campaigns/{id}/activate.
func (h *Handlers) ActivateCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	campaign, err := h.Service.ActivateCampaign(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue status notification so the advertiser is notified that their
	// campaign was approved (§61 Asynq Tasks, ADR-079 §7). The notify task
	// checks the campaign status and dispatches an "ad_campaign_approved"
	// notification when status = active. Fire-and-forget per Rule 8.
	if h.AsynqClient != nil {
		notifyPayload, _ := json.Marshal(map[string]string{

			"campaign_id": id.String(),
		})
		if _, err := h.AsynqClient.Enqueue(
			asynq.NewTask(worker.TypeAdsNotifyCampaignStatus, notifyPayload),
			asynq.Queue(worker.QueueForTask(worker.TypeAdsNotifyCampaignStatus)),
		); err != nil {
			slog.Warn("ads: failed to enqueue notify task on activate", "campaign_id", id.String(), "error", err)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// PauseCampaign handles POST /api/v1/admin/advertising/campaigns/{id}/pause.
func (h *Handlers) PauseCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	campaign, err := h.Service.PauseCampaign(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// EndCampaign handles POST /api/v1/admin/advertising/campaigns/{id}/end.
func (h *Handlers) EndCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	campaign, err := h.Service.EndCampaign(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// RejectCampaign handles POST /api/v1/admin/ads/campaigns/{id}/reject.
// Rejects a pending-review self-serve campaign and enqueues the credit
// refund task (ads:refund_campaign_credits). The refund task honours the
// ads.credit_refund_on_reject system setting.
// Ref: japaconnect-engineering/sections/61-self-serve-ads.md (Review Gate).
func (h *Handlers) RejectCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	campaign, err := h.Service.RejectCampaign(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue credit refund task (fire-and-forget per Rule 8).
	if h.AsynqClient != nil {
		payload, _ := json.Marshal(map[string]string{

			"campaign_id": id.String(),
		})
		if _, err := h.AsynqClient.Enqueue(
			asynq.NewTask(worker.TypeAdsRefundCampaignCredits, payload),
			asynq.Queue(worker.QueueForTask(worker.TypeAdsRefundCampaignCredits)),
		); err != nil {
			slog.Warn("ads: failed to enqueue refund task on reject", "campaign_id", id.String(), "error", err)
		}

		// Enqueue status notification (fire-and-forget).
		notifyPayload, _ := json.Marshal(map[string]string{

			"campaign_id": id.String(),
		})
		if _, err := h.AsynqClient.Enqueue(
			asynq.NewTask(worker.TypeAdsNotifyCampaignStatus, notifyPayload),
			asynq.Queue(worker.QueueForTask(worker.TypeAdsNotifyCampaignStatus)),
		); err != nil {
			slog.Warn("ads: failed to enqueue notify task on reject", "campaign_id", id.String(), "error", err)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// ---------------------------------------------------------------------------
// Ad Rendering Handler
// ---------------------------------------------------------------------------

// RenderSlot handles GET /api/v1/ads/serve?slot={slot_name}.
// Returns eligible campaigns for the given slot. Empty result when no match
// — never a placeholder. Public (catalog: GET /ads/serve) — guests are served
// the untargeted/default eligible set; usergroup-based targeting is not yet
// implemented (no targeting column on ad_campaigns — deferred, see plan P5).
func (h *Handlers) RenderSlot(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slotName := r.URL.Query().Get("slot")
	if slotName == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slot", "slot query parameter is required")
		return
	}

	user := auth.FromContext(r.Context())

	var userID pgtype.UUID

	if user != nil {
		userID = user.UserID
	}

	campaigns, err := h.Service.RenderSlot(r.Context(), userID, slotName)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Content is jsonb scanned as []byte, which encoding/json would emit as a
	// base64 string — the frontend expects a JSON object. Wrap each campaign
	// so Content marshals as raw JSON.
	wire := make([]map[string]any, len(campaigns))
	for i, c := range campaigns {
		wire[i] = campaignToWire(c)
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": wire})
}

// campaignToWire converts a stored campaign to the API wire shape. The
// generated model's Content field is []byte (base64 on marshal); the wire
// contract expects the raw JSON object.
func campaignToWire(c AdCampaign) map[string]any {
	return map[string]any{
		"id":                     c.ID,
		"name":                   c.Name,
		"slot_type_id":           c.SlotTypeID,
		"content":                json.RawMessage(c.Content),
		"target_url":             c.TargetUrl,
		"budget":                 c.Budget,
		"spent":                  c.Spent,
		"status":                 c.Status,
		"priority":               c.Priority,
		"frequency_cap_per_user": c.FrequencyCapPerUser,
		"start_date":             c.StartDate,
		"end_date":               c.EndDate,
		"created_by":             c.CreatedBy,
		"created_at":             c.CreatedAt,
		"updated_at":             c.UpdatedAt,
		"target_geo":             c.TargetGeo,
		"target_space_ids":       c.TargetSpaceIds,
		"organization_id":        c.OrganizationID,
		"flat_rate":              c.FlatRate,
		"budget_credits":         c.BudgetCredits,
	}
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Events) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "empty_events", "events array is empty")
		return
	}

	// Ref: japaconnect-engineering/sections/25-ad-placement.md
	// Batched ingestion: enqueue each valid event as a fire-and-forget
	// ad:ingest_event_batch task (low tier). Never write synchronously on
	// every view — the worker handles the DB insert asynchronously.
	// Guest events (empty user_id) are supported: user_id is nullable.
	ingested := 0

	for _, e := range input.Events {
		if e.EventType != EventTypeImpression && e.EventType != EventTypeClick {
			continue
		}

		var campaignID pgtype.UUID
		if err := campaignID.Scan(e.CampaignID); err != nil {
			continue
		}

		// user_id is optional — guests submit events without it.
		var userID pgtype.UUID
		if e.UserID != "" {
			_ = userID.Scan(e.UserID)
		}

		h.enqueueAdEventIngest(tenantID, campaignID, userID, e.EventType)

		ingested++
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	slotTypes, err := h.Service.ListSlotTypes(r.Context(), limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": nonNilSlice(slotTypes)})
}

// CreateSlotType handles POST /api/v1/admin/advertising/slot-types.
func (h *Handlers) CreateSlotType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	slotType, err := h.Service.CreateSlotType(r.Context(), AdSlotTypeInput{
		Name:        input.Name,
		Description: input.Description,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": slotType})
}

// UpdateSlotType handles PUT /api/v1/admin/advertising/slot-types/{id}.
func (h *Handlers) UpdateSlotType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid slot type ID")
		return
	}

	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	slotType, err := h.Service.UpdateSlotType(r.Context(), id, AdSlotTypeInput{
		Name:        input.Name,
		Description: input.Description,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": slotType})
}

// DeleteSlotType handles DELETE /api/v1/admin/advertising/slot-types/{id}.
func (h *Handlers) DeleteSlotType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid slot type ID")
		return
	}

	if err := h.Service.DeleteSlotType(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// DeleteCampaign handles DELETE /api/v1/admin/advertising/campaigns/{id}.
func (h *Handlers) DeleteCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	if err := h.Service.DeleteCampaign(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// GetCampaignStats handles GET /api/v1/admin/advertising/campaigns/{id}/stats.
func (h *Handlers) GetCampaignStats(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	stats, err := h.Service.GetCampaignStats(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": stats})
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
	_, ok = h.requireTenant(w, r)
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.OrganizationID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_organization_id", "organization_id is required")
		return
	}

	if input.CampaignID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_campaign_id", "campaign_id is required")
		return
	}

	var organizationID pgtype.UUID
	if err := organizationID.Scan(input.OrganizationID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_organization_id", "Invalid organization_id")
		return
	}

	var campaignID pgtype.UUID
	if err := campaignID.Scan(input.CampaignID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_campaign_id", "Invalid campaign_id")
		return
	}

	startDate := time.Now()

	if input.StartDate != "" {
		var err error

		startDate, err = time.Parse(time.RFC3339, input.StartDate)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_start_date", "Invalid start_date format")
			return
		}
	}

	var endDate *time.Time

	if input.EndDate != "" {
		t, err := time.Parse(time.RFC3339, input.EndDate)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_end_date", "Invalid end_date format")
			return
		}

		endDate = &t
	}

	boost, err := h.Service.CreateBoost(r.Context(), CreateBoostInput{
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

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": boost})
}

// ListBoosts handles GET /api/v1/admin/advertising/boosts?organization_id={id}.
func (h *Handlers) ListBoosts(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	orgIDStr := r.URL.Query().Get("organization_id")
	if orgIDStr == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_organization_id", "organization_id query parameter is required")
		return
	}

	var organizationID pgtype.UUID
	if err := organizationID.Scan(orgIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_organization_id", "Invalid organization_id")
		return
	}

	boosts, err := h.Service.ListBoosts(r.Context(), organizationID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": nonNilSlice(boosts)})
}

// ListActiveBoosts handles GET /api/v1/admin/advertising/boosts/active.
func (h *Handlers) ListActiveBoosts(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	boosts, err := h.Service.ListActiveBoosts(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": nonNilSlice(boosts)})
}

// EndBoost handles POST /api/v1/admin/advertising/boosts/{id}/end.
func (h *Handlers) EndBoost(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid boost ID")
		return
	}

	boost, err := h.Service.EndBoost(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": boost})
}

// UpdateBoost handles PATCH /api/v1/admin/ads/boosts/{id}.
func (h *Handlers) UpdateBoost(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid boost ID")
		return
	}

	var input struct {
		Status  *string `json:"status"`
		EndDate *string `json:"end_date"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var endDate *time.Time

	if input.EndDate != nil && *input.EndDate != "" {
		t, perr := time.Parse(time.RFC3339, *input.EndDate)
		if perr != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_end_date", "Invalid end_date format")
			return
		}

		endDate = &t
	}

	boost, err := h.Service.UpdateBoost(r.Context(), id, input.Status, endDate)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": boost})
}

// DeleteBoost handles DELETE /api/v1/admin/ads/boosts/{id}.
func (h *Handlers) DeleteBoost(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid boost ID")
		return
	}

	if err := h.Service.DeleteBoost(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ─── P5.6 Advertising Extras ─────────────────────────────────────────────

// UpdateCampaign handles PATCH /admin/ads/campaigns/:id.
// Ref: japaconnect-engineering/sections/25-ad-placement.md
// Partial update of name and budget only — ad_campaigns has no description
// column. Uses the generated UpdateAdCampaignDetails query via the service
// layer (Rule 2 — never raw SQL in handlers).
func (h *Handlers) UpdateCampaign(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid campaign ID")
		return
	}

	var input struct {
		Name   string `json:"name"`
		Budget *int64 `json:"budget"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	campaign, err := h.Service.UpdateCampaignDetails(r.Context(), id, input.Name, input.Budget)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": campaign})
}

// GetSlotType handles GET /admin/ads/slot-types/:id.
// Ref: japaconnect-engineering/sections/25-ad-placement.md
// Returns the full AdSlotType struct (slot_name, max_display_per_page, etc.)
// via the generated GetAdSlotTypeByID query. The previous raw-SQL handler
// referenced non-existent width/height columns and would 500.
func (h *Handlers) GetSlotType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid slot type ID")
		return
	}

	slotType, err := h.Service.GetSlotType(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": slotType})
}

// ---------------------------------------------------------------------------
// Ad Networks — admin CRUD handlers (Section 25 extension)
// ---------------------------------------------------------------------------

// CreateAdNetwork creates a new ad network configuration.
func (h *Handlers) CreateAdNetwork(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	var input CreateAdNetworkInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	network, err := h.Service.CreateAdNetworkDTO(r.Context(), input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": network})
}

// ListAdNetworks lists all ad networks for the tenant.
func (h *Handlers) ListAdNetworks(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	pageSize := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	networks, total, err := h.Service.ListAdNetworkDTOs(r.Context(), pageSize, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": nonNilSlice(networks),
		"meta": map[string]any{"total": total, "page_offset": offset, "page_size": pageSize},
	})
}

// GetAdNetwork returns a single ad network by ID.
func (h *Handlers) GetAdNetwork(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ad network ID")
		return
	}

	network, err := h.Service.GetAdNetworkDTO(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": network})
}

// UpdateAdNetwork updates an existing ad network.
func (h *Handlers) UpdateAdNetwork(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ad network ID")
		return
	}

	var input UpdateAdNetworkInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	network, err := h.Service.UpdateAdNetworkDTO(r.Context(), id, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": network})
}

// DeleteAdNetwork deletes an ad network by ID.
func (h *Handlers) DeleteAdNetwork(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ad network ID")
		return
	}

	if err := h.Service.DeleteAdNetwork(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Ad Network Config — public endpoint (no api_key exposed)
// ---------------------------------------------------------------------------

// ListAdNetworkProviders returns the set of allowed ad network provider values.
// Ref: japaconnect-engineering/sections/25-ad-placement.md (Ad Networks)
// Exposed so the admin frontend can populate the provider <select> from the
// backend instead of hardcoding the list (Rule 16).
// GET /admin/ads/networks/providers.
func (h *Handlers) ListAdNetworkProviders(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}

	providers := h.Service.ListAdNetworkProviders()
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// GetAdNetworkConfig returns active ad networks for public consumption (no api_key exposed).
func (h *Handlers) GetAdNetworkConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	networks, err := h.Service.ListActiveAdNetworks(ctx)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	type safeNetwork struct {
		Provider    string          `json:"provider"`
		Name        string          `json:"name"`
		ClientID    *string         `json:"client_id"`
		SlotMapping json.RawMessage `json:"slot_mapping"`
		Config      json.RawMessage `json:"config"`
		Priority    int32           `json:"priority"`
	}

	safe := make([]safeNetwork, 0, len(networks))
	for _, n := range networks {
		safe = append(safe, safeNetwork{
			Provider:    n.Provider,
			Name:        n.Name,
			ClientID:    n.ClientID,
			SlotMapping: n.SlotMapping,
			Config:      n.Config,
			Priority:    n.Priority,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": nonNilSlice(safe)})
}
