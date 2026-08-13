package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meilisearch/meilisearch-go"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/websocket"
	"github.com/jedafe/japa/internal/worker"
)

// HonchoHealthChecker checks Honcho service connectivity for the Control
// Panel infrastructure health group (ADR-027). The honcho.Client satisfies
// this interface.
type HonchoHealthChecker interface {
	Health(ctx context.Context) error
}

// Handlers holds dependencies for control panel HTTP handlers.
type Handlers struct {
	Service        *Service
	DB             *db.DB
	FeatureChecker *features.Checker
	SpamStore      *spam.Store
	PublicAPI      *publicapi.Handlers
	Inspector      *asynq.Inspector
	Client         *asynq.Client
	RBAC           *rbac.RBAC
	Settings       *config.Settings
	Valkey         valkeycompat.Cmdable
	MeiliSearch    meilisearch.ServiceManager
	WSHub          *websocket.Hub
	Honcho         HonchoHealthChecker
	AIStore        *ai.Store
	ProviderStore  *ai.ProviderStore
	AIDispatcher   *ai.Dispatcher
	StartTime      time.Time
}

// NewHandlers creates a new Handlers.
func NewHandlers(service *Service, database *db.DB, featureChecker *features.Checker, spamStore *spam.Store, publicAPI *publicapi.Handlers) *Handlers {
	return &Handlers{
		Service:        service,
		DB:             database,
		FeatureChecker: featureChecker,
		SpamStore:      spamStore,
		PublicAPI:      publicAPI,
		StartTime:      time.Now(),
	}
}

// SetInspector injects the Asynq inspector for queue monitoring.
func (h *Handlers) SetInspector(inspector *asynq.Inspector) {
	h.Inspector = inspector
}

// SetClient injects the Asynq client used to enqueue cron/scheduler tasks on demand.
func (h *Handlers) SetClient(client *asynq.Client) {
	h.Client = client
}

// SetRBAC injects the RBAC resolver for cache operations.
func (h *Handlers) SetRBAC(r *rbac.RBAC) {
	h.RBAC = r
}

// SetSettings injects the settings service for cache invalidation.
func (h *Handlers) SetSettings(s *config.Settings) {
	h.Settings = s
}

// SetValkey injects the Valkey client for cache stats.
func (h *Handlers) SetValkey(v valkeycompat.Cmdable) {
	h.Valkey = v
}

// SetMeiliSearch injects the Meilisearch client for health checks.
func (h *Handlers) SetMeiliSearch(m meilisearch.ServiceManager) {
	h.MeiliSearch = m
}

// SetWSHub injects the WebSocket hub for health checks.
func (h *Handlers) SetWSHub(hub *websocket.Hub) {
	h.WSHub = hub
}

// SetHoncho injects the Honcho health checker for the infrastructure health
// group (ADR-027).
func (h *Handlers) SetHoncho(hc HonchoHealthChecker) {
	h.Honcho = hc
}

// SetAIStore injects the AI usage store for the AI budget dashboard
// (GET/PATCH /control/ai-budget). When nil, spend defaults to 0.
func (h *Handlers) SetAIStore(s *ai.Store) {
	h.AIStore = s
}

// SetProviderStore injects the AI provider config store for the AI Providers
// management page (CRUD /control/ai-providers). When nil, the list endpoint
// returns an empty array.
func (h *Handlers) SetProviderStore(s *ai.ProviderStore) {
	h.ProviderStore = s
}

// SetAIDispatcher injects the AI dispatcher so provider config changes can
// hot-reload the dispatcher at runtime (create/update/delete operations
// call reloadAIProviders which re-reads configs from the DB).
func (h *Handlers) SetAIDispatcher(d *ai.Dispatcher) {
	h.AIDispatcher = d
}

// ---------------------------------------------------------------------------
// Tenant Handlers (delegate to Service)
// ---------------------------------------------------------------------------

func (h *Handlers) ListTenants(w http.ResponseWriter, r *http.Request) {
	// Parse pagination params (§17: filter + sort + paginate, no infinite scroll).
	limit := int32(20)
	offset := int32(0)

	if s := r.URL.Query().Get("page_size"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 32); err == nil && n > 0 {
			limit = int32(n)
			if limit > 100 {
				limit = 100
			}
		}
	}

	if s := r.URL.Query().Get("page_offset"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 32); err == nil && n >= 0 {
			offset = int32(n)
		}
	}

	statusFilter := r.URL.Query().Get("status")

	tenants, err := h.Service.ListTenants(r.Context(), statusFilter, limit, offset)
	if err != nil {
		slog.Error("control: list tenants", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list tenants")
		return
	}

	total, err := h.Service.CountTenants(r.Context(), statusFilter)
	if err != nil {
		slog.Error("control: count tenants", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to count tenants")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":   tenants,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handlers) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name   string  `json:"name"`
		Slug   string  `json:"slug"`
		Domain *string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Name == "" || input.Slug == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "name and slug are required")
		return
	}
	tenant, err := h.Service.CreateTenant(r.Context(), CreateTenantInput{
		Name:   input.Name,
		Slug:   input.Slug,
		Domain: input.Domain,
	})
	if err != nil {
		slog.Error("control: create tenant", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create tenant")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": tenant})
}

func (h *Handlers) GetTenant(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tenant ID")
		return
	}
	tenant, err := h.Service.GetTenant(r.Context(), id)
	if err != nil {
		slog.Error("control: get tenant", "error", err)
		writeError(w, http.StatusNotFound, "not_found", "Tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tenant})
}

func (h *Handlers) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tenant ID")
		return
	}
	var input struct {
		Name   string  `json:"name"`
		Domain *string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	tenant, err := h.Service.UpdateTenant(r.Context(), id, UpdateTenantInput{
		Name:   input.Name,
		Domain: input.Domain,
	})
	if err != nil {
		slog.Error("control: update tenant", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update tenant")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tenant})
}

func (h *Handlers) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tenant ID")
		return
	}
	tenant, err := h.Service.DeleteTenant(r.Context(), id)
	if err != nil {
		slog.Error("control: delete tenant", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete tenant")
		return
	}
	h.auditTenantAction(r, "tenant.delete", id)

	// Enqueue the tenant:cleanup_data task with a 30-day delay so all
	// non-cascading child data is hard-deleted after the grace period.
	// The initiated_by field carries the admin's user ID for traceability.
	if h.Client != nil {
		initiatedBy := "system"
		if u := auth.FromContext(r.Context()); u != nil {
			initiatedBy = u.UserID.String()
		}
		payload, _ := json.Marshal(map[string]string{"tenant_id": tenant.ID.String(), "initiated_by": initiatedBy})
		task := asynq.NewTask(worker.TypeTenantCleanupData, payload)
		if _, err := h.Client.Enqueue(task,
			asynq.ProcessAt(time.Now().Add(30*24*time.Hour)),
			asynq.Queue(worker.QueueForTask(worker.TypeTenantCleanupData)),
		); err != nil {
			slog.Error("control: enqueue cleanup task", "error", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tenant})
}

func (h *Handlers) SuspendTenant(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tenant ID")
		return
	}
	tenant, err := h.Service.SuspendTenant(r.Context(), id)
	if err != nil {
		slog.Error("control: suspend tenant", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to suspend tenant")
		return
	}
	h.auditTenantAction(r, "tenant.suspend", id)
	writeJSON(w, http.StatusOK, map[string]any{"data": tenant})
}

func (h *Handlers) UnsuspendTenant(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tenant ID")
		return
	}
	tenant, err := h.Service.UnsuspendTenant(r.Context(), id)
	if err != nil {
		slog.Error("control: unsuspend tenant", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unsuspend tenant")
		return
	}
	h.auditTenantAction(r, "tenant.unsuspend", id)
	writeJSON(w, http.StatusOK, map[string]any{"data": tenant})
}

// ---------------------------------------------------------------------------
// System Settings Handlers
// ---------------------------------------------------------------------------

func (h *Handlers) ListSystemSettings(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	rows, err := h.DB.Reader.Query(r.Context(),
		`SELECT id, key, value, is_public, created_at, updated_at FROM system_settings WHERE tenant_id = $1 ORDER BY key`,
		tenantID,
	)
	if err != nil {
		slog.Error("control: list system settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list settings")
		return
	}
	defer rows.Close()

	type enrichedSetting struct {
		ID           pgtype.UUID `json:"id"`
		Key          string      `json:"key"`
		Value        string      `json:"value"`
		IsPublic     bool        `json:"is_public"`
		CreatedAt    time.Time   `json:"created_at"`
		UpdatedAt    time.Time   `json:"updated_at"`
		Type         string      `json:"type"`
		DefaultValue string      `json:"default_value"`
		Description  string      `json:"description"`
		Namespace    string      `json:"namespace"`
		Panel        string      `json:"panel"`
		Min          float64     `json:"min,omitempty"`
		Max          float64     `json:"max,omitempty"`
		Sensitive    bool        `json:"sensitive"`
		Options      []any       `json:"options,omitempty"`
	}
	var settings []enrichedSetting
	for rows.Next() {
		var s enrichedSetting
		if err := rows.Scan(&s.ID, &s.Key, &s.Value, &s.IsPublic, &s.CreatedAt, &s.UpdatedAt); err != nil {
			slog.Error("control: scan setting", "error", err)
			continue
		}
		// Enrich with authoritative metadata from the settings catalog.
		meta := getSettingMeta(s.Key, s.Value)
		s.Type = meta.Type
		s.DefaultValue = meta.DefaultValue
		s.Description = meta.Description
		s.Sensitive = meta.Sensitive
		s.Min = meta.Min
		s.Max = meta.Max
		s.Panel = meta.Panel
		// Derive namespace from the first dot segment.
		if idx := strings.IndexByte(s.Key, '.'); idx >= 0 {
			s.Namespace = s.Key[:idx]
		}
		// Parse options JSON if present.
		if meta.Options != "" {
			var opts []any
			if err := json.Unmarshal([]byte(meta.Options), &opts); err == nil {
				s.Options = opts
			}
		}
		settings = append(settings, s)
	}
	// Also include catalog entries that are not yet stored in the DB (so the
	// frontend can see all available settings even before they have been saved).
	storedKeys := make(map[string]bool, len(settings))
	for _, s := range settings {
		storedKeys[s.Key] = true
	}
	for key, meta := range settingsCatalog {
		if storedKeys[key] {
			continue
		}
		var ns string
		if idx := strings.IndexByte(key, '.'); idx >= 0 {
			ns = key[:idx]
		}
		var opts []any
		if meta.Options != "" {
			json.Unmarshal([]byte(meta.Options), &opts)
		}
		settings = append(settings, enrichedSetting{
			Key:          key,
			Value:        meta.DefaultValue,
			Type:         meta.Type,
			DefaultValue: meta.DefaultValue,
			Description:  meta.Description,
			Namespace:    ns,
			Panel:        meta.Panel,
			Min:          meta.Min,
			Max:          meta.Max,
			Sensitive:    meta.Sensitive,
			Options:      opts,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": settings})
}

func (h *Handlers) UpdateSystemSettings(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	var input map[string]any
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	for key, value := range input {
		// The system_settings.value column is jsonb, so every value must be
		// valid JSON. json.Marshal produces the canonical encoding for each
		// Go type: strings get wrapped in quotes ("3"), booleans become true,
		// numbers stay bare, and arrays/objects are serialized as-is.
		//
		// However, when the frontend sends a JSON array/object as a string
		// (e.g. the onboarding.promoted_* settings store '["uuid"]' as a
		// string), json.Marshal would double-encode it to '"[\\"uuid\\"]"'.
		// Detect this case and store the raw JSON directly.
		var data []byte
		if s, ok := value.(string); ok {
			trimmed := strings.TrimSpace(s)
			if (strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) ||
				(strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) {
				if json.Valid([]byte(trimmed)) {
					data = []byte(trimmed)
				} else {
					data, _ = json.Marshal(value)
				}
			} else {
				data, _ = json.Marshal(value)
			}
		} else {
			data, _ = json.Marshal(value)
		}
		// Consult the catalog for the is_public flag. On INSERT, set is_public
		// from the catalog so appearance/branding settings are exposed via
		// GET /site/settings/public. On CONFLICT (existing row), preserve the
		// current is_public value — an admin may have explicitly toggled it.
		isPublic := false
		if meta, ok := settingsCatalog[key]; ok {
			isPublic = meta.Public
		}
		_, err := h.DB.Writer.Exec(r.Context(),
			`INSERT INTO system_settings (tenant_id, key, value, is_public, updated_at) VALUES ($1, $2, $3, $4, NOW()) ON CONFLICT (tenant_id, key) DO UPDATE SET value = $3, updated_at = NOW()`,
			tenantID, key, string(data), isPublic,
		)
		if err != nil {
			slog.Error("control: update system setting", "key", key, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update setting: "+key)
			return
		}
	}
	if h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context(), tenantID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

func (h *Handlers) UpdateSystemSetting(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "Setting key is required")
		return
	}
	var input struct {
		Value    any   `json:"value"`
		IsPublic *bool `json:"is_public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	// The value column is jsonb — json.Marshal produces the correct JSON
	// encoding for each type (strings quoted, booleans bare, etc.).
	data, _ := json.Marshal(input.Value)
	// Resolve is_public: explicit request body value takes precedence; fall
	// back to the catalog default so appearance/branding settings are public.
	isPublic := false
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	} else if meta, ok := settingsCatalog[key]; ok {
		isPublic = meta.Public
	}
	_, err := h.DB.Writer.Exec(r.Context(),
		`INSERT INTO system_settings (tenant_id, key, value, is_public, updated_at) VALUES ($1, $2, $3, $4, NOW()) ON CONFLICT (tenant_id, key) DO UPDATE SET value = $3, is_public = $4, updated_at = NOW()`,
		tenantID, key, string(data), isPublic,
	)
	if err != nil {
		slog.Error("control: update system setting", "key", key, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update setting")
		return
	}
	if h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context(), tenantID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "status": "updated"}})
}

func (h *Handlers) ResetSystemSetting(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "Setting key is required")
		return
	}
	tag, err := h.DB.Writer.Exec(r.Context(),
		`DELETE FROM system_settings WHERE tenant_id = $1 AND key = $2`,
		tenantID, key,
	)
	if err != nil {
		slog.Error("control: reset system setting", "key", key, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to reset setting")
		return
	}
	if h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context(), tenantID)
	}
	// Look up the default value from the catalog; if not found, the setting was
	// deleted but had no catalog entry — return an empty default.
	defaultValue := ""
	if meta, ok := settingsCatalog[key]; ok {
		defaultValue = meta.DefaultValue
	}
	if tag.RowsAffected() == 0 {
		// Setting didn't exist in DB (already at default). Return the default value.
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "value": defaultValue, "default_value": defaultValue}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "value": defaultValue, "default_value": defaultValue}})
}

// ---------------------------------------------------------------------------
// Per-Tenant Settings Handlers
// ---------------------------------------------------------------------------

// GetTenantSettings handles GET /control/tenants/:id/settings.
func (h *Handlers) GetTenantSettings(w http.ResponseWriter, r *http.Request) {
	tenantID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tenant ID")
		return
	}
	rows, err := h.DB.Reader.Query(r.Context(),
		`SELECT id, key, value, is_public, created_at, updated_at FROM system_settings WHERE tenant_id = $1 ORDER BY key`,
		tenantID,
	)
	if err != nil {
		slog.Error("control: get tenant settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list settings")
		return
	}
	defer rows.Close()

	type enrichedSetting struct {
		ID           pgtype.UUID `json:"id"`
		Key          string      `json:"key"`
		Value        string      `json:"value"`
		IsPublic     bool        `json:"is_public"`
		CreatedAt    time.Time   `json:"created_at"`
		UpdatedAt    time.Time   `json:"updated_at"`
		Type         string      `json:"type"`
		DefaultValue string      `json:"default_value"`
		Description  string      `json:"description"`
		Namespace    string      `json:"namespace"`
		Panel        string      `json:"panel"`
		Min          float64     `json:"min,omitempty"`
		Max          float64     `json:"max,omitempty"`
		Sensitive    bool        `json:"sensitive"`
		Options      []any       `json:"options,omitempty"`
	}
	var settings []enrichedSetting
	for rows.Next() {
		var s enrichedSetting
		if err := rows.Scan(&s.ID, &s.Key, &s.Value, &s.IsPublic, &s.CreatedAt, &s.UpdatedAt); err != nil {
			slog.Error("control: scan tenant setting", "error", err)
			continue
		}
		meta := getSettingMeta(s.Key, s.Value)
		s.Type = meta.Type
		s.DefaultValue = meta.DefaultValue
		s.Description = meta.Description
		s.Sensitive = meta.Sensitive
		s.Min = meta.Min
		s.Max = meta.Max
		s.Panel = meta.Panel
		if idx := strings.IndexByte(s.Key, '.'); idx >= 0 {
			s.Namespace = s.Key[:idx]
		}
		if meta.Options != "" {
			var opts []any
			if err := json.Unmarshal([]byte(meta.Options), &opts); err == nil {
				s.Options = opts
			}
		}
		settings = append(settings, s)
	}
	// Include catalog entries not yet stored in DB.
	storedKeys := make(map[string]bool, len(settings))
	for _, s := range settings {
		storedKeys[s.Key] = true
	}
	for key, meta := range settingsCatalog {
		if storedKeys[key] {
			continue
		}
		var ns string
		if idx := strings.IndexByte(key, '.'); idx >= 0 {
			ns = key[:idx]
		}
		var opts []any
		if meta.Options != "" {
			json.Unmarshal([]byte(meta.Options), &opts)
		}
		settings = append(settings, enrichedSetting{
			Key:          key,
			Value:        meta.DefaultValue,
			Type:         meta.Type,
			DefaultValue: meta.DefaultValue,
			Description:  meta.Description,
			Namespace:    ns,
			Panel:        meta.Panel,
			Min:          meta.Min,
			Max:          meta.Max,
			Sensitive:    meta.Sensitive,
			Options:      opts,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": settings})
}

// UpdateTenantSettings handles PATCH /control/tenants/:id/settings (bulk).
func (h *Handlers) UpdateTenantSettings(w http.ResponseWriter, r *http.Request) {
	tenantID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tenant ID")
		return
	}
	var input map[string]any
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	for key, value := range input {
		// The system_settings.value column is jsonb, so every value must be
		// valid JSON. When the frontend sends a JSON array/object as a string
		// (e.g. onboarding.promoted_* settings store '["uuid"]' as a string),
		// json.Marshal would double-encode it. Detect this case and store the
		// raw JSON directly — mirrors UpdateSystemSettings logic.
		var data []byte
		if s, ok := value.(string); ok {
			trimmed := strings.TrimSpace(s)
			if (strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) ||
				(strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) {
				if json.Valid([]byte(trimmed)) {
					data = []byte(trimmed)
				} else {
					data, _ = json.Marshal(value)
				}
			} else {
				data, _ = json.Marshal(value)
			}
		} else {
			data, _ = json.Marshal(value)
		}
		// Consult the catalog for the is_public flag so appearance/branding
		// settings are exposed via GET /site/settings/public. On CONFLICT
		// (existing row), preserve the current is_public value.
		isPublic := false
		if meta, ok := settingsCatalog[key]; ok {
			isPublic = meta.Public
		}
		_, err := h.DB.Writer.Exec(r.Context(),
			`INSERT INTO system_settings (tenant_id, key, value, is_public, updated_at) VALUES ($1, $2, $3, $4, NOW()) ON CONFLICT (tenant_id, key) DO UPDATE SET value = $3, updated_at = NOW()`,
			tenantID, key, string(data), isPublic,
		)
		if err != nil {
			slog.Error("control: update tenant setting", "key", key, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update setting: "+key)
			return
		}
	}
	if h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context(), tenantID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// UpdateTenantSettingKey handles PATCH /control/tenants/:id/settings/:key.
func (h *Handlers) UpdateTenantSettingKey(w http.ResponseWriter, r *http.Request) {
	tenantID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tenant ID")
		return
	}
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "Setting key is required")
		return
	}
	var input struct {
		Value    any   `json:"value"`
		IsPublic *bool `json:"is_public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	data, _ := json.Marshal(input.Value)
	if input.IsPublic != nil {
		_, err := h.DB.Writer.Exec(r.Context(),
			`INSERT INTO system_settings (tenant_id, key, value, is_public, updated_at) VALUES ($1, $2, $3, $4, NOW()) ON CONFLICT (tenant_id, key) DO UPDATE SET value = $3, is_public = $4, updated_at = NOW()`,
			tenantID, key, string(data), *input.IsPublic,
		)
		if err != nil {
			slog.Error("control: update tenant setting key", "key", key, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update setting")
			return
		}
	} else {
		_, err := h.DB.Writer.Exec(r.Context(),
			`INSERT INTO system_settings (tenant_id, key, value, updated_at) VALUES ($1, $2, $3, NOW()) ON CONFLICT (tenant_id, key) DO UPDATE SET value = $3, updated_at = NOW()`,
			tenantID, key, string(data),
		)
		if err != nil {
			slog.Error("control: update tenant setting key", "key", key, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update setting")
			return
		}
	}
	if h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context(), tenantID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "status": "updated"}})
}

// ResetTenantSetting handles POST /control/tenants/:id/settings/:key/reset.
func (h *Handlers) ResetTenantSetting(w http.ResponseWriter, r *http.Request) {
	tenantID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid tenant ID")
		return
	}
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "Setting key is required")
		return
	}
	tag, err := h.DB.Writer.Exec(r.Context(),
		`DELETE FROM system_settings WHERE tenant_id = $1 AND key = $2`,
		tenantID, key,
	)
	if err != nil {
		slog.Error("control: reset tenant setting", "key", key, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to reset setting")
		return
	}
	if h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context(), tenantID)
	}
	defaultValue := ""
	if meta, ok := settingsCatalog[key]; ok {
		defaultValue = meta.DefaultValue
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "value": defaultValue, "default_value": defaultValue}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "value": defaultValue, "default_value": defaultValue}})
}

// ---------------------------------------------------------------------------
// Feature Flags Handlers — removed.
// Feature flags are now managed exclusively via the admin panel's
// /admin/feature-flags routes (internal/features/handlers.go), which provide
// full CRUD with metadata. Super-admins with control.panel.access also have
// admin.feature_flags.manage, so they use the admin panel for flag management.
// The control panel's /extensions page handles tool/extension toggles via
// system_settings — a separate concern from feature flags.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Infrastructure Health Handlers
// ---------------------------------------------------------------------------

func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type serviceStatus struct {
		Name      string `json:"name"`
		Status    string `json:"status"`
		LatencyMs int64  `json:"latency_ms"`
		Message   string `json:"message,omitempty"`
	}

	checkService := func(name string, fn func(context.Context) error) serviceStatus {
		start := time.Now()
		err := fn(ctx)
		latency := time.Since(start).Milliseconds()
		if err != nil {
			return serviceStatus{Name: name, Status: "down", LatencyMs: latency, Message: err.Error()}
		}
		return serviceStatus{Name: name, Status: "healthy", LatencyMs: latency}
	}

	infrastructure := []serviceStatus{
		checkService("PostgreSQL", func(ctx context.Context) error {
			return h.DB.Ping(ctx)
		}),
	}

	if h.Valkey != nil {
		infrastructure = append(infrastructure, checkService("Valkey", func(ctx context.Context) error {
			_, err := h.Valkey.Ping(ctx).Result()
			return err
		}))
	}

	// Honcho agent-memory service (ADR-027: Control Panel infrastructure
	// health group — connectivity, same pattern as Meilisearch/Valkey).
	if h.Honcho != nil {
		infrastructure = append(infrastructure, checkService("Honcho", func(ctx context.Context) error {
			return h.Honcho.Health(ctx)
		}))
	}

	application := []serviceStatus{
		checkService("API Server", func(_ context.Context) error {
			return nil
		}),
	}

	if h.MeiliSearch != nil {
		application = append(application, checkService("Meilisearch", func(ctx context.Context) error {
			_, err := h.MeiliSearch.HealthWithContext(ctx)
			return err
		}))
	}

	background := []serviceStatus{}

	if h.Inspector != nil {
		background = append(background, checkService("Asynq Workers", func(ctx context.Context) error {
			info, err := h.Inspector.GetQueueInfo("default")
			if err != nil {
				return err
			}
			if info.Size < 0 {
				return nil
			}
			return nil
		}))
		background = append(background, checkService("Asynq Scheduler", func(ctx context.Context) error {
			_, err := h.Inspector.SchedulerEntries()
			return err
		}))
	}

	realtime := []serviceStatus{
		checkService("WebSocket Hub", func(_ context.Context) error {
			if h.WSHub == nil {
				return nil
			}
			_ = h.WSHub.GetOnlineUsers()
			return nil
		}),
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"infrastructure": infrastructure,
			"application":    application,
			"background":     background,
			"realtime":       realtime,
		},
	})
}

func (h *Handlers) DBHealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check both pools: Writer is always the primary; Reader is the replica
	// when JC_DATABASE_READER_URL is set, otherwise it is also the primary.
	// This lets the control panel show the status of both nodes.
	start := time.Now()
	readerErr := h.DB.Reader.Ping(ctx)
	readerLatency := time.Since(start).Milliseconds()

	start = time.Now()
	writerErr := h.DB.Writer.Ping(ctx)
	writerLatency := time.Since(start).Milliseconds()

	if readerErr != nil && writerErr != nil {
		slog.Error("control: db health check", "reader_error", readerErr, "writer_error", writerErr)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"data": map[string]any{"status": "unhealthy", "error": readerErr.Error()},
		})
		return
	}

	if readerErr != nil {
		slog.Warn("control: db reader pool unreachable", "error", readerErr)
	}
	if writerErr != nil {
		slog.Warn("control: db writer pool unreachable", "error", writerErr)
	}

	latency := readerLatency
	if readerErr != nil {
		latency = writerLatency
	}

	// Database version.
	var version string
	_ = h.DB.Reader.QueryRow(ctx, `SELECT version()`).Scan(&version)

	// Database size.
	var sizeBytes int64
	_ = h.DB.Reader.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&sizeBytes)

	// Table count and index count.
	var tableCount, indexCount int
	_ = h.DB.Reader.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE')::int,
			(SELECT count(*) FROM pg_indexes WHERE schemaname = 'public')::int
	`).Scan(&tableCount, &indexCount)

	// Table details — includes dead_tuple_ratio from pg_stat_user_tables
	// (Ref: japaconnect-engineering/sections/21-control-panel.md — Database Health Tooling).
	type tableInfo struct {
		Name           string  `json:"name"`
		RowCount       int64   `json:"row_count"`
		SizeBytes      int64   `json:"size_bytes"`
		IndexCount     int     `json:"index_count"`
		LastVacuum     *string `json:"last_vacuum"`
		DeadTupleRatio float64 `json:"dead_tuple_ratio"`
	}
	var tables []tableInfo
	rows, err := h.DB.Reader.Query(ctx, `
		SELECT
			t.tablename,
			COALESCE(c.reltuples::bigint, 0),
			pg_table_size(c.oid),
			(SELECT count(*) FROM pg_indexes WHERE tablename = t.tablename AND schemaname = 'public')::int,
			(SELECT max(last_vacuum)::text FROM pg_stat_user_tables WHERE relname = t.tablename),
			COALESCE(
				(SELECT n_dead_tup::float / NULLIF(n_live_tup + n_dead_tup, 0)
				 FROM pg_stat_user_tables WHERE relname = t.tablename),
				0
			)
		FROM pg_tables t
		LEFT JOIN pg_class c ON c.relname = t.tablename
		WHERE t.schemaname = 'public'
		ORDER BY pg_table_size(c.oid) DESC
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var ti tableInfo
			if err := rows.Scan(&ti.Name, &ti.RowCount, &ti.SizeBytes, &ti.IndexCount, &ti.LastVacuum, &ti.DeadTupleRatio); err == nil {
				tables = append(tables, ti)
			}
		}
	}
	if tables == nil {
		tables = []tableInfo{}
	}

	// Table bloat — top 10 tables by dead tuple ratio (heuristic proxy for
	// bloat without requiring the pgstattuple extension).
	// Ref: japaconnect-engineering/sections/21-control-panel.md
	type bloatInfo struct {
		Name           string  `json:"name"`
		DeadTupleRatio float64 `json:"dead_tuple_ratio"`
		SizeBytes      int64   `json:"size_bytes"`
	}
	var tableBloat []bloatInfo
	for _, t := range tables {
		if t.DeadTupleRatio > 0 {
			tableBloat = append(tableBloat, bloatInfo{
				Name:           t.Name,
				DeadTupleRatio: t.DeadTupleRatio,
				SizeBytes:      t.SizeBytes,
			})
		}
	}
	// Sort by dead_tuple_ratio descending, take top 10.
	for i := 0; i < len(tableBloat); i++ {
		for j := i + 1; j < len(tableBloat); j++ {
			if tableBloat[j].DeadTupleRatio > tableBloat[i].DeadTupleRatio {
				tableBloat[i], tableBloat[j] = tableBloat[j], tableBloat[i]
			}
		}
	}
	if len(tableBloat) > 10 {
		tableBloat = tableBloat[:10]
	}
	if tableBloat == nil {
		tableBloat = []bloatInfo{}
	}

	// Dead tuple ratio — tables with dead_tuple_ratio > 5%.
	type deadTupleRatioInfo struct {
		Name           string  `json:"name"`
		DeadTupleRatio float64 `json:"dead_tuple_ratio"`
	}
	var deadTupleRatios []deadTupleRatioInfo
	for _, t := range tables {
		if t.DeadTupleRatio > 0.05 {
			deadTupleRatios = append(deadTupleRatios, deadTupleRatioInfo{
				Name:           t.Name,
				DeadTupleRatio: t.DeadTupleRatio,
			})
		}
	}
	if deadTupleRatios == nil {
		deadTupleRatios = []deadTupleRatioInfo{}
	}

	// Unused indexes.
	type unusedIndex struct {
		Name      string `json:"name"`
		TableName string `json:"table_name"`
		Scans     int64  `json:"scans"`
	}
	var indexesUnused []unusedIndex
	irows, err := h.DB.Reader.Query(ctx, `
		SELECT indexrelname, relname, idx_scan
		FROM pg_stat_user_indexes
		WHERE idx_scan = 0 AND schemaname = 'public'
		ORDER BY pg_relation_size(indexrelid) DESC
	`)
	if err == nil {
		defer irows.Close()
		for irows.Next() {
			var ui unusedIndex
			if err := irows.Scan(&ui.Name, &ui.TableName, &ui.Scans); err == nil {
				indexesUnused = append(indexesUnused, ui)
			}
		}
	}
	if indexesUnused == nil {
		indexesUnused = []unusedIndex{}
	}

	// Connection pool stats (server-side session counts from pg_stat_activity).
	var activeConns, idleConns, maxConns int
	_ = h.DB.Reader.QueryRow(ctx, `
		SELECT
			COALESCE((SELECT count(*) FROM pg_stat_activity WHERE state = 'active' AND datname = current_database()), 0),
			COALESCE((SELECT count(*) FROM pg_stat_activity WHERE state = 'idle' AND datname = current_database()), 0),
			COALESCE((SELECT setting::int FROM pg_settings WHERE name = 'max_connections'), 0)
	`).Scan(&activeConns, &idleConns, &maxConns)

	// Replication.
	type replicationInfo struct {
		State      string  `json:"state"`
		LagSeconds float64 `json:"lag_seconds"`
	}
	var repl *replicationInfo
	var isRecovery bool
	var replLag float64
	err = h.DB.Reader.QueryRow(ctx, `
		SELECT
			pg_is_in_recovery(),
			COALESCE(EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()))::float, 0)
	`).Scan(&isRecovery, &replLag)
	if err == nil {
		state := "primary"
		if isRecovery {
			state = "standby"
		}
		repl = &replicationInfo{State: state, LagSeconds: replLag}
	}

	// Slow queries (active queries running > 1s).
	type slowQuery struct {
		PID          int    `json:"pid"`
		Duration     int64  `json:"duration"`
		DurationMs   int64  `json:"duration_ms"`
		State        string `json:"state"`
		QueryPreview string `json:"query_preview"`
		Query        string `json:"query"`
		Timestamp    string `json:"timestamp"`
	}
	var slowQueries []slowQuery
	srows, err := h.DB.Reader.Query(ctx, `
		SELECT
			pid,
			EXTRACT(EPOCH FROM (now() - query_start))::bigint,
			EXTRACT(EPOCH FROM (now() - query_start))::bigint * 1000,
			COALESCE(state, 'unknown'),
			LEFT(query, 100),
			query,
			query_start::text
		FROM pg_stat_activity
		WHERE state = 'active'
			AND query NOT LIKE '%pg_stat_activity%'
			AND now() - query_start > interval '1 second'
		ORDER BY query_start ASC
		LIMIT 20
	`)
	if err == nil {
		defer srows.Close()
		for srows.Next() {
			var sq slowQuery
			if err := srows.Scan(&sq.PID, &sq.Duration, &sq.DurationMs, &sq.State, &sq.QueryPreview, &sq.Query, &sq.Timestamp); err == nil {
				slowQueries = append(slowQueries, sq)
			}
		}
	}
	if slowQueries == nil {
		slowQueries = []slowQuery{}
	}

	// Average query time from pg_stat_statements (if the extension is enabled).
	// Falls back to 0 when the extension is not installed.
	var avgQueryTimeMs float64
	_ = h.DB.Reader.QueryRow(ctx, `
		SELECT COALESCE(avg(mean_exec_time), 0)
		FROM pg_stat_statements
		WHERE query NOT LIKE '%pg_stat_statements%'
	`).Scan(&avgQueryTimeMs)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"connected":         readerErr == nil || writerErr == nil,
			"version":           version,
			"latency_ms":        latency,
			"size_bytes":        sizeBytes,
			"table_count":       tableCount,
			"index_count":       indexCount,
			"avg_query_time_ms": avgQueryTimeMs,
			"tables":            tables,
			"table_bloat":       tableBloat,
			"dead_tuple_ratio":  deadTupleRatios,
			"slow_queries":      slowQueries,
			"indexes_unused":    indexesUnused,
			"connection_pool": map[string]any{
				"active": activeConns,
				"idle":   idleConns,
				"max":    maxConns,
			},
			"pools": map[string]any{
				"primary": map[string]any{
					"role":       "primary",
					"reachable":  writerErr == nil,
					"latency_ms": writerLatency,
				},
				"reader": map[string]any{
					"role":       "reader",
					"reachable":  readerErr == nil,
					"latency_ms": readerLatency,
				},
			},
			"replication": repl,
		},
	})
}

func (h *Handlers) DBVacuum(w http.ResponseWriter, r *http.Request) {
	// Parse optional JSON body: { table_name?: string, full?: bool }
	// When omitted (no body or empty object), the entire database is
	// vacuumed (table_name=""). A malformed body is rejected with 400 so
	// a bad request does not silently trigger a full-database VACUUM.
	var payload struct {
		Version   int    `json:"version"` // payload schema version (current: 1)
		TableName string `json:"table_name"`
		Full      bool   `json:"full"`
	}
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&payload); err != nil {
			// io.EOF means an empty body — that's fine, use zero values.
			if !errors.Is(err, io.EOF) {
				writeError(w, http.StatusBadRequest, "invalid_request_body", "Malformed JSON body")
				return
			}
		}
	}

	// Enqueue async vacuum task via Asynq for background processing.
	if h.Client != nil {
		payload.Version = 1 // current payload schema version
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to encode vacuum payload")
			return
		}
		task := asynq.NewTask(worker.TypeDBManualVacuum, payloadBytes)
		if _, err := h.Client.Enqueue(task, asynq.Queue(worker.QueueForTask(worker.TypeDBManualVacuum))); err != nil {
			slog.Error("control: failed to enqueue db vacuum task", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to enqueue vacuum task")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "enqueued"}})
		return
	}

	// Fallback: run inline if no Asynq client. Use a detached context so
	// the VACUUM is not cancelled by the request deadline (VACUUM on large
	// tables can exceed the 30s WriteTimeout).
	vacuumCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Minute)
	defer cancel()
	if err := h.DB.Vacuum(vacuumCtx, payload.TableName, payload.Full); err != nil {
		slog.Error("control: db vacuum", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to run VACUUM")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "completed"}})
}

// ---------------------------------------------------------------------------
// Queue Monitor Handlers (wired to Asynq inspector — BE-040)
// ---------------------------------------------------------------------------

func (h *Handlers) QueueStats(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"queues":              []any{},
				"total_pending":       0,
				"total_active":        0,
				"total_completed_24h": 0,
				"total_failed_24h":    0,
				"dlq_count":           0,
			},
		})
		return
	}

	type queueInfo struct {
		Name         string  `json:"name"`
		Pending      int     `json:"pending"`
		Active       int     `json:"active"`
		Completed24H int     `json:"completed_24h"`
		Failed24H    int     `json:"failed_24h"`
		LatencyMs    float64 `json:"latency_ms"`
	}

	var queues []queueInfo
	var totalPending, totalActive, totalCompleted, totalFailed, dlqDepth int

	for _, queueName := range []string{worker.QueueCritical, worker.QueueDefault, worker.QueueLow} {
		qi, err := h.Inspector.GetQueueInfo(queueName)
		if err != nil {
			// Queue may not exist yet (Asynq creates lazily on first enqueue).
			// Show it as an empty queue rather than silently skipping.
			slog.Debug("queue stats: queue not found, showing as empty", "queue", queueName)
			queues = append(queues, queueInfo{
				Name: queueName,
			})
			continue
		}
		// Asynq's Processed counter resets daily and includes both
		// succeeded and failed tasks. Completed (stored) is the number
		// of tasks still in the completed state within retention — not
		// a daily count. Use Processed-Failed for the daily success count.
		completed24h := qi.Processed - qi.Failed
		if completed24h < 0 {
			completed24h = 0
		}
		queues = append(queues, queueInfo{
			Name:         queueName,
			Pending:      qi.Pending,
			Active:       qi.Active,
			Completed24H: completed24h,
			Failed24H:    qi.Failed,
			LatencyMs:    qi.Latency.Seconds() * 1000,
		})
		totalPending += qi.Pending
		totalActive += qi.Active
		totalCompleted += completed24h
		totalFailed += qi.Failed
		dlqDepth += qi.Archived
	}

	if queues == nil {
		queues = []queueInfo{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"queues":              queues,
			"total_pending":       totalPending,
			"total_active":        totalActive,
			"total_completed_24h": totalCompleted,
			"total_failed_24h":    totalFailed,
			"dlq_count":           dlqDepth,
		},
	})
}

func (h *Handlers) ListDLQ(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	tasks := make([]map[string]any, 0)

	for _, queueName := range []string{"critical", "default", "low"} {
		entries, err := h.Inspector.ListArchivedTasks(queueName)
		if err != nil {
			slog.Debug("dlq: list tasks failed (queue may not exist yet)", "queue", queueName, "error", err)
			continue
		}
		for _, entry := range entries {
			tasks = append(tasks, map[string]any{
				"task_id":       entry.ID,
				"task_type":     entry.Type,
				"queue":         queueName,
				"payload":       string(entry.Payload),
				"error_message": entry.LastErr,
				"retry_count":   entry.Retried,
				"max_retries":   entry.MaxRetry,
				"failed_at":     entry.LastFailedAt.Format(time.RFC3339),
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tasks})
}

// ListQueueTasks returns recent tasks for a specific queue across all states
// (pending, active, scheduled, retry, archived, completed).
func (h *Handlers) ListQueueTasks(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"pending": []any{}, "active": []any{}, "scheduled": []any{},
			"retry": []any{}, "archived": []any{}, "completed": []any{},
		}})
		return
	}

	queueName := chi.URLParam(r, "queue_name")
	if queueName == "" {
		writeError(w, http.StatusBadRequest, "missing_queue", "queue_name is required")
		return
	}

	type taskSummary struct {
		ID            string  `json:"id"`
		Type          string  `json:"type"`
		State         string  `json:"state"`
		Payload       string  `json:"payload"`
		RetryCount    int     `json:"retry_count"`
		MaxRetry      int     `json:"max_retry"`
		LastErr       string  `json:"last_err,omitempty"`
		LastFailedAt  *string `json:"last_failed_at,omitempty"`
		NextProcessAt *string `json:"next_process_at,omitempty"`
	}

	summarize := func(tasks []*asynq.TaskInfo, state string) []taskSummary {
		result := make([]taskSummary, 0, len(tasks))
		for _, t := range tasks {
			ts := taskSummary{
				ID:         t.ID,
				Type:       t.Type,
				State:      state,
				Payload:    string(t.Payload),
				RetryCount: t.Retried,
				MaxRetry:   t.MaxRetry,
			}
			if t.LastErr != "" {
				ts.LastErr = t.LastErr
			}
			if !t.LastFailedAt.IsZero() {
				s := t.LastFailedAt.Format(time.RFC3339)
				ts.LastFailedAt = &s
			}
			if !t.NextProcessAt.IsZero() {
				s := t.NextProcessAt.Format(time.RFC3339)
				ts.NextProcessAt = &s
			}
			result = append(result, ts)
		}
		return result
	}

	// Collect tasks from each state, best-effort (skip non-existent queues).
	pending, _ := h.Inspector.ListPendingTasks(queueName)
	active, _ := h.Inspector.ListActiveTasks(queueName)
	scheduled, _ := h.Inspector.ListScheduledTasks(queueName)
	retry, _ := h.Inspector.ListRetryTasks(queueName)
	archived, _ := h.Inspector.ListArchivedTasks(queueName)
	completed, _ := h.Inspector.ListCompletedTasks(queueName)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"pending":   summarize(pending, "pending"),
			"active":    summarize(active, "active"),
			"scheduled": summarize(scheduled, "scheduled"),
			"retry":     summarize(retry, "retry"),
			"archived":  summarize(archived, "archived"),
			"completed": summarize(completed, "completed"),
		},
	})
}

func (h *Handlers) RetryDLQTask(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		writeError(w, http.StatusServiceUnavailable, "no_inspector", "Inspector not available")
		return
	}

	taskID := chi.URLParam(r, "task_id")

	for _, queueName := range []string{"critical", "default", "low"} {
		if err := h.Inspector.RunTask(queueName, taskID); err != nil {
			continue
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": taskID, "status": "retried"}})
		return
	}

	writeError(w, http.StatusNotFound, "task_not_found", "Task not found in any DLQ")
}

// RetryQueueTask retries a specific task within a named queue.
// Uses asynq.Inspector.RunTask to move a retry/archived task back to
// pending. Wired at POST /control/queue/{queue_name}/tasks/{task_id}/retry.
func (h *Handlers) RetryQueueTask(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		writeError(w, http.StatusServiceUnavailable, "no_inspector", "Inspector not available")
		return
	}

	queueName := chi.URLParam(r, "queue_name")
	if queueName == "" {
		writeError(w, http.StatusBadRequest, "missing_queue", "queue_name is required")
		return
	}

	taskID := chi.URLParam(r, "task_id")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "missing_task", "task_id is required")
		return
	}

	if err := h.Inspector.RunTask(queueName, taskID); err != nil {
		writeError(w, http.StatusNotFound, "task_not_found", "Task not found in queue")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": taskID, "status": "retried"}})
}

func (h *Handlers) DiscardDLQTask(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		writeError(w, http.StatusServiceUnavailable, "no_inspector", "Inspector not available")
		return
	}

	taskID := chi.URLParam(r, "task_id")

	for _, queueName := range []string{"critical", "default", "low"} {
		if err := h.Inspector.DeleteTask(queueName, taskID); err != nil {
			continue
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": taskID, "status": "discarded"}})
		return
	}

	writeError(w, http.StatusNotFound, "task_not_found", "Task not found in any DLQ")
}

func (h *Handlers) DiscardAllDLQ(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		writeError(w, http.StatusServiceUnavailable, "no_inspector", "Inspector not available")
		return
	}

	totalDiscarded := 0

	for _, queueName := range []string{"critical", "default", "low"} {
		entries, err := h.Inspector.ListArchivedTasks(queueName)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if err := h.Inspector.DeleteTask(queueName, entry.ID); err == nil {
				totalDiscarded++
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"discarded": totalDiscarded}})
}

// ---------------------------------------------------------------------------
// Cache Management Handlers
// ---------------------------------------------------------------------------

type cacheNamespace struct {
	Prefix           string  `json:"prefix"`
	KeyCount         int64   `json:"key_count"`
	MemoryEstimateMB float64 `json:"memory_estimate_mb"`
}

type cacheStatsResponse struct {
	Namespaces    []cacheNamespace `json:"namespaces"`
	TotalKeys     int64            `json:"total_keys"`
	TotalMemoryMB float64          `json:"total_memory_mb"`
}

func (h *Handlers) CacheStats(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"message": "Valkey client not configured",
			},
		})
		return
	}

	ctx := r.Context()
	info, err := h.Valkey.Info(ctx, "memory").Result()
	if err != nil {
		slog.Error("control: cache stats", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch cache stats")
		return
	}

	totalMemoryMB := parseValkeyMemory(info)

	namespaces := countKeysByPrefix(ctx, h.Valkey)

	var totalKeys int64
	for _, ns := range namespaces {
		totalKeys += ns.KeyCount
	}

	if totalKeys > 0 {
		for i := range namespaces {
			namespaces[i].MemoryEstimateMB = (float64(namespaces[i].KeyCount) / float64(totalKeys)) * totalMemoryMB
		}
	}

	stats := cacheStatsResponse{
		Namespaces:    namespaces,
		TotalKeys:     totalKeys,
		TotalMemoryMB: totalMemoryMB,
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": stats})
}

func parseValkeyMemory(info string) float64 {
	var totalMemoryBytes int64

	lines := strings.Split(info, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "used_memory:") {
			if val, err := strconv.ParseInt(strings.TrimPrefix(line, "used_memory:"), 10, 64); err == nil {
				totalMemoryBytes = val
			}
			break
		}
	}

	return float64(totalMemoryBytes) / (1024 * 1024)
}

func countKeysByPrefix(ctx context.Context, vk valkeycompat.Cmdable) []cacheNamespace {
	if vk == nil {
		return nil
	}

	prefixes := []struct {
		name    string
		pattern string
	}{
		{"jc:perm", "jc:perm:*"},
		{"jc:settings", "jc:settings:*"},
		{"jc:cache:page", "jc:cache:page:*"},
		{"jc:cache:feed", "jc:cache:feed:*"},
		{"jc:cache:i18n", "jc:cache:i18n:*"},
		{"jc:cache:search", "jc:cache:search:*"},
		{"jc:cache:analytics", "jc:cache:analytics:*"},
		{"jc:ratelimit", "jc:ratelimit:*"},
		{"jc:idempotency", "jc:idempotency:*"},
		{"jc:session", "jc:session:*"},
		{"jc:spam", "jc:spam:*"},
		{"jc:render", "jc:render:*"},
		{"jc:link_preview", "jc:link_preview:*"},
		{"jc:redirect", "jc:redirect:*"},
		{"jc:editor_lock", "jc:editor_lock:*"},
		{"jc:api_rate", "jc:api_rate:*"},
		{"jc:ws", "jc:ws*"},
		{"jc:other", "jc:*"},
	}

	var namespaces []cacheNamespace
	seen := make(map[string]bool)

	for _, p := range prefixes {
		var cursor uint64
		var count int64

		for {
			keys, nextCursor, err := vk.Scan(ctx, cursor, p.pattern, 100).Result()
			if err != nil {
				slog.Error("control: cache stats scan", "pattern", p.pattern, "error", err)
				break
			}

			for _, key := range keys {
				if p.name == "jc:other" {
					isOther := true
					for _, pp := range prefixes {
						if pp.name != "jc:other" && strings.HasPrefix(key, strings.TrimSuffix(pp.pattern, "*")) {
							isOther = false
							break
						}
					}
					if isOther && !seen[key] {
						count++
						seen[key] = true
					}
				} else if !seen[key] {
					count++
					seen[key] = true
				}
			}

			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}

		if count > 0 || p.name != "jc:other" {
			namespaces = append(namespaces, cacheNamespace{
				Prefix:           p.name,
				KeyCount:         count,
				MemoryEstimateMB: 0,
			})
		}
	}

	return namespaces
}

func (h *Handlers) FlushCache(w http.ResponseWriter, r *http.Request) {
	if h.RBAC != nil {
		h.RBAC.InvalidateAllCache(r.Context())
	}
	tenantID, ok := requireTenant(w, r)
	if ok && h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context(), tenantID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"message": "Permission and settings cache flushed"},
	})
}

// ---------------------------------------------------------------------------
// Security Handlers
// ---------------------------------------------------------------------------

func (h *Handlers) SecurityOverview(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	blocks, err := h.SpamStore.GetActiveIPBlocks(r.Context(), tenantID)
	if err != nil {
		slog.Error("control: security overview", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch security overview")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"active_ip_blocks": len(blocks),
		},
	})
}

func (h *Handlers) ListIPBlocks(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	blocks, err := h.SpamStore.ListIPBlocks(r.Context(), tenantID, 200, pgtype.Timestamptz{})
	if err != nil {
		slog.Error("control: list ip blocks", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list IP blocks")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": blocks})
}

func (h *Handlers) ListSecurityScanResults(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 0 {
		page = 0
	}
	limit := 20
	offset := int32(page * limit)

	severity := r.URL.Query().Get("severity")

	var results []map[string]any
	var total int64

	countErr := h.DB.Reader.QueryRow(r.Context(),
		`SELECT count(*) FROM security_scan_results WHERE tenant_id = $1 AND ($2::text = '' OR severity = $2)`,
		tenantID, severity,
	).Scan(&total)
	if countErr != nil {
		slog.Warn("control: count security scan results", "error", countErr)
	}

	rows, err := h.DB.Reader.Query(r.Context(),
		`SELECT id, scan_type, severity, finding, meta, resolved_at, created_at
		 FROM security_scan_results
		 WHERE tenant_id = $1 AND ($2::text = '' OR severity = $2)
		 ORDER BY created_at DESC, id DESC
		 LIMIT $3 OFFSET $4`,
		tenantID, severity, int32(limit), offset,
	)
	if err != nil {
		slog.Error("control: list security scan results", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list security scan results")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id pgtype.UUID
		var scanType, sev, finding string
		var meta []byte
		var resolvedAt pgtype.Timestamptz
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&id, &scanType, &sev, &finding, &meta, &resolvedAt, &createdAt); err != nil {
			slog.Error("control: scan result row", "error", err)
			continue
		}
		results = append(results, map[string]any{
			"id":          id.String(),
			"severity":    sev,
			"title":       finding,
			"description": finding,
			"scan_type":   scanType,
			"meta":        string(meta),
			"resolved":    resolvedAt.Valid,
			"created_at":  createdAt.Time.Format(time.RFC3339),
		})
	}
	if results == nil {
		results = []map[string]any{}
	}

	totalPages := int(total) / limit
	if int(total)%limit > 0 {
		totalPages++
	}
	if totalPages < 1 {
		totalPages = 1
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"results":     results,
			"page":        page,
			"total_pages": totalPages,
		},
	})
}

func (h *Handlers) TriggerSecurityScan(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.Client == nil {
		writeError(w, http.StatusServiceUnavailable, "queue_unavailable", "Task queue not available")
		return
	}

	payload, err := json.Marshal(map[string]any{
		"tenant_id": tenantID.String(),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create scan task")
		return
	}

	// AUDIT-BE-TASK-008: use the constant worker.TypeSecurityScan instead of
	// the hardcoded string "security:scan" (DRY — prevents silent task drops
	// if the constant value changes).
	task := asynq.NewTask(worker.TypeSecurityScan, payload)
	info, err := h.Client.Enqueue(task)
	if err != nil {
		slog.Error("control: trigger security scan", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to enqueue security scan")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"task_id": info.ID,
			"status":  "enqueued",
		},
	})
}

func (h *Handlers) CreateIPBlock(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	var input struct {
		CIDR      string  `json:"cidr"`
		BlockType string  `json:"block_type"`
		Reason    *string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.CIDR == "" || input.BlockType == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "cidr and block_type are required")
		return
	}
	block, err := h.SpamStore.CreateIPBlock(r.Context(), spam.CreateIPBlockInput{
		TenantID:  tenantID,
		CIDR:      input.CIDR,
		BlockType: input.BlockType,
		Reason:    input.Reason,
		BlockedBy: userID,
	})
	if err != nil {
		slog.Error("control: create ip block", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create IP block")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": block})
}

func (h *Handlers) DeleteIPBlock(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid IP block ID")
		return
	}
	if err := h.SpamStore.DeleteIPBlock(r.Context(), id, tenantID); err != nil {
		slog.Error("control: delete ip block", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete IP block")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// AI Budget Handlers (stubs)
// ---------------------------------------------------------------------------

// aiBudgetThresholdKey is the system_settings key holding the monthly AI spend
// alert threshold in USD (see §19). It is an alert threshold, not a hard cap.
const aiBudgetThresholdKey = "ai.monthly_budget_usd"

// parseBudgetUSD decodes a JSON-encoded system_settings value into a float USD
// amount, tolerating a bare number ("100"), a JSON number (100), or a JSON
// string ("100"). Returns 0 when the value is missing or unparseable.
func parseBudgetUSD(v string) float64 {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "\"")
	if v == "" {
		return 0
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	var f float64
	if json.Unmarshal([]byte(v), &f) == nil {
		return f
	}
	return 0
}

// GetAIBudget handles GET /api/v1/control/ai-budget.
// Returns the current AI budget summary. The monthly threshold is read from
// system_settings (ai.monthly_budget_usd). Month-to-date spend and per-provider
// / per-task breakdowns aggregate from the ai_usage_log table via the AI store.
// alert_triggered is derived from threshold vs spend.
func (h *Handlers) GetAIBudget(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	threshold := 0.0
	if h.Settings != nil {
		if v, err := h.Settings.GetSetting(r.Context(), tenantID, aiBudgetThresholdKey); err == nil {
			threshold = parseBudgetUSD(v)
		}
	}

	// Aggregate month-to-date spend from ai_usage_log. When the AI store is
	// not wired (e.g. tests), fall back to 0 spend with empty breakdowns.
	monthToDateCost := 0.0
	byProvider := []any{}
	byTaskType := []any{}
	if h.AIStore != nil {
		summary, err := h.AIStore.GetAIUsageSummary(r.Context(), tenantID, 30, "")
		if err != nil {
			slog.Error("control: get ai budget: usage summary", "error", err)
		} else {
			monthToDateCost = summary.TotalCostUSD
			for _, b := range summary.ByProvider {
				byProvider = append(byProvider, map[string]any{
					"provider":    b.Provider,
					"tokens_used": b.TokensUsed,
					"cost_usd":    b.TotalCostUSD,
					"calls":       b.Calls,
				})
			}
			for _, b := range summary.ByTaskType {
				byTaskType = append(byTaskType, map[string]any{
					"task_type":   b.TaskType,
					"tokens_used": b.TokensUsed,
					"cost_usd":    b.TotalCostUSD,
					"call_count":  b.Calls,
				})
			}
		}
	}

	alertTriggered := threshold > 0 && monthToDateCost >= threshold

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"month_to_date_cost":   monthToDateCost,
			"budget_threshold_usd": threshold,
			"alert_triggered":      alertTriggered,
			"by_provider":          byProvider,
			"by_task_type":         byTaskType,
		},
	})
}

// UpdateAIBudget handles PATCH /api/v1/control/ai-budget.
// Persists the monthly alert threshold (ai.monthly_budget_usd) to
// system_settings and returns the updated budget summary.
func (h *Handlers) UpdateAIBudget(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		BudgetThresholdUSD *float64 `json:"budget_threshold_usd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.BudgetThresholdUSD == nil || *input.BudgetThresholdUSD < 0 {
		writeError(w, http.StatusBadRequest, "invalid_threshold", "budget_threshold_usd must be a non-negative number")
		return
	}

	threshold := *input.BudgetThresholdUSD
	data, _ := json.Marshal(threshold)
	if _, err := h.DB.Writer.Exec(r.Context(),
		`INSERT INTO system_settings (tenant_id, key, value, updated_at) VALUES ($1, $2, $3, NOW()) ON CONFLICT (tenant_id, key) DO UPDATE SET value = $3, updated_at = NOW()`,
		tenantID, aiBudgetThresholdKey, string(data),
	); err != nil {
		slog.Error("control: update ai budget", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update AI budget threshold")
		return
	}
	if h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context(), tenantID)
	}

	// Re-aggregate spend so the response reflects the current state.
	monthToDateCost := 0.0
	byProvider := []any{}
	byTaskType := []any{}
	if h.AIStore != nil {
		summary, err := h.AIStore.GetAIUsageSummary(r.Context(), tenantID, 30, "")
		if err != nil {
			slog.Error("control: update ai budget: usage summary", "error", err)
		} else {
			monthToDateCost = summary.TotalCostUSD
			for _, b := range summary.ByProvider {
				byProvider = append(byProvider, map[string]any{
					"provider":    b.Provider,
					"tokens_used": b.TokensUsed,
					"cost_usd":    b.TotalCostUSD,
					"calls":       b.Calls,
				})
			}
			for _, b := range summary.ByTaskType {
				byTaskType = append(byTaskType, map[string]any{
					"task_type":   b.TaskType,
					"tokens_used": b.TokensUsed,
					"cost_usd":    b.TotalCostUSD,
					"call_count":  b.Calls,
				})
			}
		}
	}

	alertTriggered := threshold > 0 && monthToDateCost >= threshold

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"month_to_date_cost":   monthToDateCost,
			"budget_threshold_usd": threshold,
			"alert_triggered":      alertTriggered,
			"by_provider":          byProvider,
			"by_task_type":         byTaskType,
		},
	})
}

// ---------------------------------------------------------------------------
// Audit Log Handlers
// ---------------------------------------------------------------------------

func (h *Handlers) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	type auditLogActor struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	type auditLog struct {
		ID                 pgtype.UUID     `json:"id"`
		Timestamp          time.Time       `json:"timestamp"`
		Actor              *auditLogActor  `json:"actor"`
		ActionType         string          `json:"action_type"`
		EntityType         *string         `json:"entity_type"`
		EntityID           *string         `json:"entity_id"`
		TenantID           pgtype.UUID     `json:"tenant_id"`
		IPAddress          *string         `json:"ip_address"`
		ImpersonatedUserID *string         `json:"impersonated_user_id"`
		Metadata           json.RawMessage `json:"metadata"`
		UserAgent          *string         `json:"user_agent"`
	}

	page := 0
	perPage := 25
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v >= 0 {
			page = v
		}
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 && v <= 100 {
			perPage = v
		}
	}
	offset := page * perPage

	sortBy := "al.created_at"
	sortDir := "DESC"
	if sb := r.URL.Query().Get("sort_by"); sb != "" {
		switch sb {
		case "timestamp", "created_at":
			sortBy = "al.created_at"
		case "action", "action_type":
			sortBy = "al.action_type"
		default:
			sortBy = "al.created_at"
		}
	}
	if sd := r.URL.Query().Get("sort_dir"); sd != "" {
		sd = strings.ToUpper(sd)
		if sd == "ASC" || sd == "DESC" {
			sortDir = sd
		}
	}

	// Build WHERE clause from query filters.
	var where string
	var args []any
	argIdx := 1
	if actor := r.URL.Query().Get("actor"); actor != "" {
		where += fmt.Sprintf(" AND (al.actor_user_id::text = $%d OR u.username ILIKE '%%' || $%d || '%%')", argIdx, argIdx)
		args = append(args, actor)
		argIdx++
	}
	if actionType := r.URL.Query().Get("action_type"); actionType != "" {
		where += fmt.Sprintf(" AND al.action_type = $%d", argIdx)
		args = append(args, actionType)
		argIdx++
	}
	if entityType := r.URL.Query().Get("entity_type"); entityType != "" {
		where += fmt.Sprintf(" AND al.target_type = $%d", argIdx)
		args = append(args, entityType)
		argIdx++
	}
	if from := r.URL.Query().Get("from"); from != "" {
		where += fmt.Sprintf(" AND al.created_at >= $%d", argIdx)
		args = append(args, from)
		argIdx++
	}
	if to := r.URL.Query().Get("to"); to != "" {
		where += fmt.Sprintf(" AND al.created_at <= $%d", argIdx)
		args = append(args, to)
		argIdx++
	}
	// Impersonation-only filter (ADR-047: required at launch so the audit
	// trail is operationally useful, not write-only). When ?impersonation_only=true,
	// only rows where impersonated_user_id IS NOT NULL are returned.
	if impOnly := r.URL.Query().Get("impersonation_only"); impOnly == "true" || impOnly == "1" {
		where += " AND al.impersonated_user_id IS NOT NULL"
	}

	query := fmt.Sprintf(`SELECT al.id, al.created_at, al.actor_user_id, u.username,
		al.action_type, al.target_type, al.target_id::text, al.tenant_id,
		al.ip_address::text, al.impersonated_user_id::text, al.metadata
		FROM audit_logs al
		LEFT JOIN users u ON u.id = al.actor_user_id
		WHERE 1=1%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d`, where, sortBy, sortDir, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := h.DB.Reader.Query(r.Context(), query, args...)
	if err != nil {
		slog.Error("control: list audit logs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list audit logs")
		return
	}
	defer rows.Close()

	var logs []auditLog
	for rows.Next() {
		var l auditLog
		var actorID, username sql.NullString
		if err := rows.Scan(&l.ID, &l.Timestamp, &actorID, &username,
			&l.ActionType, &l.EntityType, &l.EntityID, &l.TenantID,
			&l.IPAddress, &l.ImpersonatedUserID, &l.Metadata); err != nil {
			slog.Error("control: scan audit log", "error", err)
			continue
		}
		if actorID.Valid {
			l.Actor = &auditLogActor{ID: actorID.String, Username: username.String}
		}
		logs = append(logs, l)
	}
	if logs == nil {
		logs = []auditLog{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": logs})
}

// auditLogDefaultRetentionDays is the fallback retention window for an
// unscoped (no-body) bulk clear. Logs older than this are purged; everything
// newer is preserved unless an explicit `confirm: true` is supplied. A
// production deployment may override this via system_settings
// `audit.logs.retention_days`.
const auditLogDefaultRetentionDays = 365

// DeleteAuditLog handles DELETE /api/v1/control/audit-logs/{id}.
// Hard-deletes a single audit log entry by id. The control panel audit log is
// a full cross-tenant super-admin view (see ListAuditLogs), so the lookup is
// not tenant-scoped. Returns 404 when the row does not exist.
func (h *Handlers) DeleteAuditLog(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid audit log ID")
		return
	}

	q := gen.New(h.DB.Writer)
	if err := q.DeleteAuditLog(r.Context(), id); err != nil {
		slog.Error("control: delete audit log", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete audit log")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"id": id.String(), "status": "deleted"},
	})
}

// AuditLogClearInput is the optional JSON body for POST /api/v1/control/audit-logs/clear.
// A missing body triggers a retention-based purge of logs older than
// auditLogDefaultRetentionDays. A `before` timestamp scopes the purge to logs
// created strictly before it; `action_type` further narrows by action. A full
// clear with no scope requires `confirm: true` to guard against accidental wipes.
type AuditLogClearInput struct {
	Before     *string `json:"before"`
	ActionType *string `json:"action_type"`
	Confirm    bool    `json:"confirm"`
}

// ClearAuditLogs handles POST /api/v1/control/audit-logs/clear.
// Bulk-deletes audit log entries. GDPR-safe by default: a no-body request
// purges only logs beyond the retention window. Scoped purges (`before` /
// `action_type`) are allowed without confirmation; an unscoped full clear
// requires an explicit `confirm: true` in the request body.
func (h *Handlers) ClearAuditLogs(w http.ResponseWriter, r *http.Request) {
	var input AuditLogClearInput
	if body, _ := io.ReadAll(r.Body); len(body) > 0 {
		if err := json.Unmarshal(body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
			return
		}
	}

	// Build the scoped WHERE clause (audit_logs is cross-tenant here).
	var where string
	var args []any

	switch {
	case input.Before != nil && *input.Before != "":
		ts, err := time.Parse(time.RFC3339, *input.Before)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_before", "before must be an RFC3339 timestamp")
			return
		}
		where = "created_at < $1"
		args = []any{ts}
		if input.ActionType != nil && *input.ActionType != "" {
			where += " AND action_type = $2"
			args = append(args, *input.ActionType)
		}
	case input.ActionType != nil && *input.ActionType != "":
		where = "action_type = $1"
		args = []any{*input.ActionType}
	default:
		// No scope: retention-based purge by default; full clear only with confirm.
		if input.Confirm {
			where = "" // clear everything
		} else {
			cutoff := time.Now().AddDate(0, 0, -auditLogDefaultRetentionDays)
			where = "created_at < $1"
			args = []any{cutoff}
		}
	}

	query := "DELETE FROM audit_logs"
	if where != "" {
		query += " WHERE " + where
	}

	tag, err := h.DB.Writer.Exec(r.Context(), query, args...)
	if err != nil {
		slog.Error("control: clear audit logs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to clear audit logs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"cleared": tag.RowsAffected()},
	})
}

// ---------------------------------------------------------------------------
// Deployment Handlers (stubs)
// ---------------------------------------------------------------------------

func (h *Handlers) ListDeployments(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"data": []map[string]any{
			{
				"version":     "1.0.0",
				"status":      "active",
				"deployed_at": h.StartTime.Format(time.RFC3339),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// API Key Management Handlers (delegate to publicapi)
// ---------------------------------------------------------------------------

func (h *Handlers) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.CreateKey(w, r)
}

func (h *Handlers) GetAPIKey(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.GetAPIKey(w, r)
}

func (h *Handlers) UpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.UpdateAPIKeyStatus(w, r)
}

func (h *Handlers) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.DeleteKeyPermanently(w, r)
}

func (h *Handlers) DeleteAPIKeysBatch(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.DeleteKeysPermanently(w, r)
}

func (h *Handlers) GetAPIKeyAnalytics(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.GetAPIKeyAnalytics(w, r)
}

// ---------------------------------------------------------------------------
// Cron / Scheduler CRUD (custom entries stored in Valkey, synced by worker)
// ---------------------------------------------------------------------------

// cronCustomKey returns the tenant-scoped Valkey key for custom cron jobs.
// Tenant-scoping (Rule 4 / §4.2 / §9) prevents one tenant's control panel from
// seeing or mutating another tenant's custom scheduler entries.
func cronCustomKey(ctx context.Context) string {
	return valkey.MustKey("cron:custom:%s", tenant.IDFromContext(ctx).String())
}

// cronPausedKey returns the tenant-scoped Valkey key for paused scheduler entries.
func cronPausedKey(ctx context.Context) string {
	return valkey.MustKey("cron:paused:%s", tenant.IDFromContext(ctx).String())
}

// cronCustomEntry is the JSON shape stored per custom cron job in Valkey.
type cronCustomEntry struct {
	Type    string `json:"type"`
	Spec    string `json:"spec"`
	Payload string `json:"payload,omitempty"`
	Paused  bool   `json:"paused,omitempty"`
}

// ListCronJobs handles GET /api/v1/control/cron/jobs.
// Merges system scheduler entries (Asynq) with admin-created custom entries (Valkey).
func (h *Handlers) ListCronJobs(w http.ResponseWriter, r *http.Request) {
	jobs := make([]map[string]any, 0)
	systemTypes := make(map[string]struct{}) // track system task types for dedup

	// 1. System entries from Asynq Scheduler
	if h.Inspector != nil {
		entries, err := h.Inspector.SchedulerEntries()
		if err == nil {
			// Fetch paused set so we can flag system entries
			var pausedSet map[string]struct{}
			if h.Valkey != nil {
				if members, err := h.Valkey.SMembers(r.Context(), cronPausedKey(r.Context())).Result(); err == nil {
					pausedSet = make(map[string]struct{}, len(members))
					for _, m := range members {
						pausedSet[m] = struct{}{}
					}
				}
			}
			for _, e := range entries {
				taskType := ""
				if e.Task != nil {
					taskType = e.Task.Type()
				}
				systemTypes[taskType] = struct{}{}
				_, paused := pausedSet[e.ID]
				jobs = append(jobs, map[string]any{
					"id":          e.ID,
					"spec":        e.Spec,
					"type":        taskType,
					"source":      "system",
					"paused":      paused,
					"next_run_at": e.Next.Format(time.RFC3339),
					"prev_run_at": e.Prev.Format(time.RFC3339),
				})
			}
		} else {
			slog.Warn("control: list cron system entries", "error", err)
		}
	}

	// 2. Custom entries from Valkey (skip if type already covered by a system entry)
	if h.Valkey != nil {
		fields, err := h.Valkey.HGetAll(r.Context(), cronCustomKey(r.Context())).Result()
		if err == nil {
			for id, raw := range fields {
				var ce cronCustomEntry
				if json.Unmarshal([]byte(raw), &ce) != nil {
					continue
				}
				if _, exists := systemTypes[ce.Type]; exists {
					continue // system entry already present
				}
				jobs = append(jobs, map[string]any{
					"id":          id,
					"spec":        ce.Spec,
					"type":        ce.Type,
					"payload":     ce.Payload,
					"source":      "custom",
					"paused":      ce.Paused,
					"next_run_at": nil,
					"prev_run_at": nil,
				})
			}
		} else {
			slog.Warn("control: list cron custom entries", "error", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": jobs})
}

// CreateCronJob handles POST /api/v1/control/cron/jobs.
// Creates a custom cron entry stored in Valkey. The worker process picks it up.
func (h *Handlers) CreateCronJob(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		writeError(w, http.StatusServiceUnavailable, "no_valkey", "Valkey not available")
		return
	}
	var input struct {
		Type    string `json:"type"`
		Spec    string `json:"spec"`
		Payload string `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Type == "" || input.Spec == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "type and spec are required")
		return
	}
	// L5: Validate payload JSON if provided (defense in depth — the
	// frontend also validates, but the backend must not accept invalid JSON).
	if input.Payload != "" {
		var js any
		if json.Unmarshal([]byte(input.Payload), &js) != nil {
			writeError(w, http.StatusBadRequest, "invalid_payload", "Payload must be valid JSON")
			return
		}
	}

	// Generate a unique ID for the custom entry
	id := fmt.Sprintf("custom_%d", time.Now().UnixNano())
	entry := cronCustomEntry{Type: input.Type, Spec: input.Spec, Payload: input.Payload}
	data, err := json.Marshal(entry)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "marshal_error", "Failed to encode entry")
		return
	}
	if err := h.Valkey.HSet(r.Context(), cronCustomKey(r.Context()), id, string(data)).Err(); err != nil {
		slog.Error("control: create cron job", "error", err)
		writeError(w, http.StatusInternalServerError, "redis_error", "Failed to save cron job")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{"id": id, "type": entry.Type, "spec": entry.Spec},
	})
}

// UpdateCronJob handles PATCH /api/v1/control/cron/jobs/{id}.
// Updates a custom cron entry in Valkey.
func (h *Handlers) UpdateCronJob(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		writeError(w, http.StatusServiceUnavailable, "no_valkey", "Valkey not available")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "Job ID is required")
		return
	}

	// Only custom entries can be edited
	raw, err := h.Valkey.HGet(r.Context(), cronCustomKey(r.Context()), id).Result()
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Cron job not found (only custom jobs can be edited)")
		return
	}
	var existing cronCustomEntry
	if json.Unmarshal([]byte(raw), &existing) != nil {
		writeError(w, http.StatusInternalServerError, "decode_error", "Failed to read existing entry")
		return
	}

	var input struct {
		Type    *string `json:"type"`
		Spec    *string `json:"spec"`
		Payload *string `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Type != nil {
		existing.Type = *input.Type
	}
	if input.Spec != nil {
		existing.Spec = *input.Spec
	}
	if input.Payload != nil {
		// L5: Validate payload JSON on update (defense in depth).
		if *input.Payload != "" {
			var js any
			if json.Unmarshal([]byte(*input.Payload), &js) != nil {
				writeError(w, http.StatusBadRequest, "invalid_payload", "Payload must be valid JSON")
				return
			}
		}
		existing.Payload = *input.Payload
	}
	if existing.Type == "" || existing.Spec == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "type and spec cannot be empty")
		return
	}

	data, err := json.Marshal(existing)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "marshal_error", "Failed to encode entry")
		return
	}
	if err := h.Valkey.HSet(r.Context(), cronCustomKey(r.Context()), id, string(data)).Err(); err != nil {
		slog.Error("control: update cron job", "error", err)
		writeError(w, http.StatusInternalServerError, "redis_error", "Failed to update cron job")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"id": id, "type": existing.Type, "spec": existing.Spec},
	})
}

// DeleteCronJob handles DELETE /api/v1/control/cron/jobs/{id}.
// Deletes a custom cron entry from Valkey or pauses a system entry (soft-delete).
func (h *Handlers) DeleteCronJob(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		writeError(w, http.StatusServiceUnavailable, "no_valkey", "Valkey not available")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "Job ID is required")
		return
	}

	// Try custom entry first
	deleted, err := h.Valkey.HDel(r.Context(), cronCustomKey(r.Context()), id).Result()
	if err != nil {
		slog.Error("control: delete cron job", "error", err)
		writeError(w, http.StatusInternalServerError, "redis_error", "Failed to delete cron job")
		return
	}
	if deleted > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "status": "deleted"}})
		return
	}

	// For system entries, we can't truly delete (managed by code).
	// Instead, mark as paused so it won't run.
	if err := h.Valkey.SAdd(r.Context(), cronPausedKey(r.Context()), id).Err(); err != nil {
		slog.Error("control: pause system cron job", "error", err)
		writeError(w, http.StatusInternalServerError, "redis_error", "Failed to pause system cron job")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"id": id, "status": "paused", "note": "System jobs cannot be deleted; entry has been paused instead."},
	})
}

// PauseCronJob handles POST /api/v1/control/cron/jobs/{id}/pause.
func (h *Handlers) PauseCronJob(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		writeError(w, http.StatusServiceUnavailable, "no_valkey", "Valkey not available")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "Job ID is required")
		return
	}

	// Check if it's a custom entry
	if raw, err := h.Valkey.HGet(r.Context(), cronCustomKey(r.Context()), id).Result(); err == nil {
		var ce cronCustomEntry
		if json.Unmarshal([]byte(raw), &ce) == nil {
			ce.Paused = true
			data, _ := json.Marshal(ce)
			if err := h.Valkey.HSet(r.Context(), cronCustomKey(r.Context()), id, string(data)).Err(); err != nil {
				slog.Error("control: pause custom cron job", "error", err)
				writeError(w, http.StatusInternalServerError, "redis_error", "Failed to pause cron job")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "status": "paused"}})
			return
		}
	}

	// System entry — add to paused set
	if err := h.Valkey.SAdd(r.Context(), cronPausedKey(r.Context()), id).Err(); err != nil {
		slog.Error("control: pause system cron job", "error", err)
		writeError(w, http.StatusInternalServerError, "redis_error", "Failed to pause cron job")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "status": "paused"}})
}

// UnpauseCronJob handles POST /api/v1/control/cron/jobs/{id}/unpause.
func (h *Handlers) UnpauseCronJob(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		writeError(w, http.StatusServiceUnavailable, "no_valkey", "Valkey not available")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "Job ID is required")
		return
	}

	// Check if it's a custom entry
	if raw, err := h.Valkey.HGet(r.Context(), cronCustomKey(r.Context()), id).Result(); err == nil {
		var ce cronCustomEntry
		if json.Unmarshal([]byte(raw), &ce) == nil {
			ce.Paused = false
			data, _ := json.Marshal(ce)
			if err := h.Valkey.HSet(r.Context(), cronCustomKey(r.Context()), id, string(data)).Err(); err != nil {
				slog.Error("control: unpause custom cron job", "error", err)
				writeError(w, http.StatusInternalServerError, "redis_error", "Failed to unpause cron job")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "status": "active"}})
			return
		}
	}

	// System entry — remove from paused set
	if err := h.Valkey.SRem(r.Context(), cronPausedKey(r.Context()), id).Err(); err != nil {
		slog.Error("control: unpause system cron job", "error", err)
		writeError(w, http.StatusInternalServerError, "redis_error", "Failed to unpause cron job")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "status": "active"}})
}

// TriggerCronJob handles POST /api/v1/control/cron/jobs/{id}/trigger.
// Enqueues the scheduler entry's task immediately (one-off run).
func (h *Handlers) TriggerCronJob(w http.ResponseWriter, r *http.Request) {
	if h.Client == nil {
		writeError(w, http.StatusServiceUnavailable, "no_client", "Asynq client not available")
		return
	}
	if h.Inspector == nil {
		writeError(w, http.StatusServiceUnavailable, "no_inspector", "Asynq inspector not available")
		return
	}

	id := chi.URLParam(r, "id")
	entries, err := h.Inspector.SchedulerEntries()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scheduler_error", "Failed to load cron jobs")
		return
	}

	for _, e := range entries {
		if e.ID != id || e.Task == nil {
			continue
		}
		if _, err := h.Client.Enqueue(e.Task, asynq.Queue(worker.QueueForTask(e.Task.Type()))); err != nil {
			writeError(w, http.StatusInternalServerError, "enqueue_error", "Failed to trigger cron job")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": e.ID, "status": "enqueued"}})
		return
	}

	// Check custom entries — trigger by re-enqueuing via Asynq client
	if h.Valkey != nil {
		if raw, err := h.Valkey.HGet(r.Context(), cronCustomKey(r.Context()), id).Result(); err == nil {
			var ce cronCustomEntry
			if json.Unmarshal([]byte(raw), &ce) == nil && ce.Type != "" {
				task := asynq.NewTask(ce.Type, []byte(ce.Payload))
				if _, err := h.Client.Enqueue(task, asynq.Queue(worker.QueueForTask(ce.Type))); err != nil {
					writeError(w, http.StatusInternalServerError, "enqueue_error", "Failed to trigger cron job")
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "status": "enqueued"}})
				return
			}
		}
	}

	writeError(w, http.StatusNotFound, "not_found", "Cron job not found")
}

// ListTaskTypes handles GET /control/task-types — returns the full Asynq task
// type catalog with name, tier, and description for the cron job editor UI.
func (h *Handlers) ListTaskTypes(w http.ResponseWriter, r *http.Request) {
	taskTypes := worker.TaskTypeCatalog()
	writeJSON(w, http.StatusOK, map[string]any{"data": taskTypes})
}

// ListSchedulePresets handles GET /control/cron/schedule-presets — returns
// the common cron schedule presets and queue tier names for the cron editor
// and backup-configs UI, so the frontend does not hardcode them (Rule 16).
func (h *Handlers) ListSchedulePresets(w http.ResponseWriter, r *http.Request) {
	presets := []map[string]string{
		{"value": "every_minute", "label": "Every minute", "cron": "* * * * *"},
		{"value": "every_5min", "label": "Every 5 minutes", "cron": "*/5 * * * *"},
		{"value": "every_15min", "label": "Every 15 minutes", "cron": "*/15 * * * *"},
		{"value": "every_30min", "label": "Every 30 minutes", "cron": "*/30 * * * *"},
		{"value": "hourly", "label": "Every hour", "cron": "0 * * * *"},
		{"value": "every_6h", "label": "Every 6 hours", "cron": "0 */6 * * *"},
		{"value": "daily_midnight", "label": "Daily at 00:00", "cron": "0 0 * * *"},
		{"value": "daily_3am", "label": "Daily at 03:00", "cron": "0 3 * * *"},
		{"value": "weekly_sun_2am", "label": "Weekly (Sun 02:00)", "cron": "0 2 * * 0"},
		{"value": "monthly_1st_2am", "label": "Monthly (1st 02:00)", "cron": "0 2 1 * *"},
		{"value": "custom", "label": "Custom cron expression...", "cron": ""},
	}
	tiers := []string{"critical", "default", "low"}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"presets": presets,
			"tiers":   tiers,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("control: json encode error", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}

// auditTenantAction writes an audit_logs row for a significant tenant
// management action (suspend, unsuspend, delete). The actor is the
// authenticated super-admin; the target is the affected tenant.
func (h *Handlers) auditTenantAction(r *http.Request, actionType string, targetID pgtype.UUID) {
	ctx := r.Context()
	t := tenant.FromContext(ctx)
	if t == nil {
		return
	}
	var actorID any
	if u := auth.FromContext(ctx); u != nil {
		actorID = u.UserID
	}
	ip := clientip.FromRequest(r)
	if _, err := h.DB.Writer.Exec(ctx, `
		INSERT INTO audit_logs (tenant_id, actor_user_id, action_type, target_type, target_id, ip_address)
		VALUES ($1, $2, $3, 'tenant', $4, $5::inet)
	`, t.ID, actorID, actionType, targetID, ip); err != nil {
		slog.Error("control: audit tenant action", "action", actionType, "error", err)
	}
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return pgtype.UUID{}, false
	}
	return u.UserID, true
}

func authUserFromContext(r *http.Request) pgtype.UUID {
	u := auth.FromContext(r.Context())
	if u == nil {
		return pgtype.UUID{}
	}
	return u.UserID
}

// ---------------------------------------------------------------------------
// Permission Nodes Handler (RBAC CRUD gap — lists all registered nodes)
// ---------------------------------------------------------------------------

func (h *Handlers) ListPermissionNodes(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Reader.Query(r.Context(),
		`SELECT key, description FROM permissions ORDER BY key`)
	if err != nil {
		slog.Error("control: list permission nodes", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list permission nodes")
		return
	}
	defer rows.Close()

	type permNode struct {
		Key         string `json:"key"`
		Description string `json:"description"`
	}
	nodes := make([]permNode, 0)
	for rows.Next() {
		var n permNode
		if err := rows.Scan(&n.Key, &n.Description); err != nil {
			continue
		}
		nodes = append(nodes, n)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": nodes})
}

// ---------------------------------------------------------------------------
// OAuth Clients Handlers (stored in system_settings as JSON under
// the key "oauth.clients" — avoids a new table migration for V1)
// ---------------------------------------------------------------------------

type oauthClientEntry struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    string `json:"created_at"`
}

// supportedOAuthProviders returns the providers the OAuth engine can actually
// instantiate (google, github, apple, facebook). Microsoft and Twitter are not
// implemented in internal/auth/oauth.go and must not be offered.
var supportedOAuthProviders = []map[string]string{
	{"key": "google", "label": "Google", "description": "Google OAuth 2.0 / OpenID Connect"},
	{"key": "github", "label": "GitHub", "description": "GitHub OAuth 2.0"},
	{"key": "apple", "label": "Apple", "description": "Sign in with Apple"},
	{"key": "facebook", "label": "Facebook", "description": "Facebook Login"},
}

func oauthClientsKey(ctx context.Context) string {
	return valkey.MustKey("oauth_clients:%s", tenant.IDFromContext(ctx).String())
}

func (h *Handlers) ListOAuthClients(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}
	raw, err := h.Valkey.Get(r.Context(), oauthClientsKey(r.Context())).Result()
	if err != nil || raw == "" {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}
	var clients []oauthClientEntry
	if json.Unmarshal([]byte(raw), &clients) != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": clients})
}

// ListOAuthProviders handles GET /control/oauth-providers — returns the list of
// OAuth providers supported by the engine (google, github, apple, facebook).
func (h *Handlers) ListOAuthProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": supportedOAuthProviders})
}

func (h *Handlers) GetOAuthClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "id is required")
		return
	}
	clients := h.loadOAuthClients(r)
	for _, c := range clients {
		if c.ID == id {
			writeJSON(w, http.StatusOK, map[string]any{"data": c})
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "OAuth client not found")
}

func (h *Handlers) CreateOAuthClient(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name         string `json:"name"`
		Provider     string `json:"provider"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RedirectURI  string `json:"redirect_uri"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Name == "" || input.Provider == "" || input.ClientID == "" || input.ClientSecret == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "name, provider, client_id, and client_secret are required")
		return
	}
	// Validate provider against the supported list.
	providerValid := false
	for _, p := range supportedOAuthProviders {
		if p["key"] == input.Provider {
			providerValid = true
			break
		}
	}
	if !providerValid {
		writeError(w, http.StatusBadRequest, "invalid_provider", "Unsupported OAuth provider: "+input.Provider)
		return
	}
	clients := h.loadOAuthClients(r)
	entry := oauthClientEntry{
		ID:           fmt.Sprintf("oauth_%d", time.Now().UnixNano()),
		Name:         input.Name,
		Provider:     input.Provider,
		ClientID:     input.ClientID,
		ClientSecret: input.ClientSecret,
		RedirectURI:  input.RedirectURI,
		Enabled:      true,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	clients = append(clients, entry)
	if err := h.saveOAuthClients(r, clients); err != nil {
		writeError(w, http.StatusInternalServerError, "save_error", "Failed to save OAuth client")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": entry})
}

func (h *Handlers) UpdateOAuthClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "id is required")
		return
	}
	var input struct {
		Name         *string `json:"name"`
		ClientSecret *string `json:"client_secret"`
		RedirectURI  *string `json:"redirect_uri"`
		Enabled      *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	clients := h.loadOAuthClients(r)
	for i := range clients {
		if clients[i].ID == id {
			if input.Name != nil {
				clients[i].Name = *input.Name
			}
			if input.ClientSecret != nil {
				clients[i].ClientSecret = *input.ClientSecret
			}
			if input.RedirectURI != nil {
				clients[i].RedirectURI = *input.RedirectURI
			}
			if input.Enabled != nil {
				clients[i].Enabled = *input.Enabled
			}
			if err := h.saveOAuthClients(r, clients); err != nil {
				writeError(w, http.StatusInternalServerError, "save_error", "Failed to update OAuth client")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": clients[i]})
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "OAuth client not found")
}

func (h *Handlers) DeleteOAuthClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "id is required")
		return
	}
	clients := h.loadOAuthClients(r)
	for i, c := range clients {
		if c.ID == id {
			clients = append(clients[:i], clients[i+1:]...)
			if err := h.saveOAuthClients(r, clients); err != nil {
				writeError(w, http.StatusInternalServerError, "save_error", "Failed to delete OAuth client")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "OAuth client not found")
}

func (h *Handlers) RotateOAuthClientSecret(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "id is required")
		return
	}
	clients := h.loadOAuthClients(r)
	for i := range clients {
		if clients[i].ID == id {
			// Generate a new random secret (32 hex chars).
			clients[i].ClientSecret = fmt.Sprintf("sec_%x", time.Now().UnixNano())
			if err := h.saveOAuthClients(r, clients); err != nil {
				writeError(w, http.StatusInternalServerError, "save_error", "Failed to rotate secret")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": id, "status": "rotated"}})
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "OAuth client not found")
}

func (h *Handlers) loadOAuthClients(r *http.Request) []oauthClientEntry {
	if h.Valkey == nil {
		return []oauthClientEntry{}
	}
	raw, err := h.Valkey.Get(r.Context(), oauthClientsKey(r.Context())).Result()
	if err != nil || raw == "" {
		return []oauthClientEntry{}
	}
	var clients []oauthClientEntry
	if json.Unmarshal([]byte(raw), &clients) != nil {
		return []oauthClientEntry{}
	}
	return clients
}

func (h *Handlers) saveOAuthClients(r *http.Request, clients []oauthClientEntry) error {
	if h.Valkey == nil {
		return fmt.Errorf("valkey not available")
	}
	data, err := json.Marshal(clients)
	if err != nil {
		return err
	}
	return h.Valkey.Set(r.Context(), oauthClientsKey(r.Context()), string(data), 0).Err()
}

// ---------------------------------------------------------------------------
// Extensions Handlers (stored in system_settings under "extensions.*")
// ---------------------------------------------------------------------------

type extensionEntry struct {
	Key              string `json:"key"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Enabled          bool   `json:"enabled"`
	Version          string `json:"version"`
	SettingKey       string `json:"setting_key"`
	SettingsCategory string `json:"settings_category"`
	// Category is used for grouping in the UI (e.g. "Content", "Social", "AI").
	Category string `json:"category"`
}

// knownExtensions is the unified catalog of all extensions and tools
// recognized by the control panel. Each entry's enabled/disabled state is
// persisted in system_settings under the key specified by SettingKey.
// SettingsCategory is the control-panel settings category the entry's title
// links to (empty = no settings page). This catalog is the single source of
// truth for all extension/tool on/off switches — the /extensions page is the
// canonical place to toggle them.
var knownExtensions = []extensionEntry{
	// Legacy extensions (stored under extensions.<key>.enabled)
	{Key: "blog", Name: "Blog", Description: "Blog series, content reviews, and featured posts", Version: "1.0.0", SettingKey: "extensions.blog.enabled", SettingsCategory: "content", Category: "Content"},
	{Key: "webhooks", Name: "Webhooks", Description: "Outgoing webhook delivery system", Version: "1.0.0", SettingKey: "extensions.webhooks.enabled", SettingsCategory: "", Category: "Integrations"},
	{Key: "ai", Name: "AI Assistant", Description: "AI-powered content assistance and moderation", Version: "1.0.0", SettingKey: "extensions.ai.enabled", SettingsCategory: "rag", Category: "AI"},
	{Key: "affiliate", Name: "Affiliate Program", Description: "Affiliate tracking and commission management", Version: "1.0.0", SettingKey: "extensions.affiliate.enabled", SettingsCategory: "", Category: "Commerce"},

	// V2 tools — each uses its own catalogued system_settings key
	// (JEOS §§60–78, ADRs 061–086)
	{Key: "rag", Name: "RAG / JapaBot Knowledge", Description: "Retrieval-augmented generation knowledge layer for JapaBot grounding", Version: "2.0.0", SettingKey: "rag.enabled", SettingsCategory: "rag", Category: "AI"},
	{Key: "studio", Name: "AI Content Studio", Description: "AI-powered content generation studio with persona-driven jobs", Version: "2.0.0", SettingKey: "studio.enabled", SettingsCategory: "studio", Category: "AI"},
	{Key: "participation", Name: "Participation Orchestration", Description: "Persona-driven participation orchestration engine", Version: "2.0.0", SettingKey: "participation.enabled", SettingsCategory: "participation", Category: "AI"},
	{Key: "checklist", Name: "Visa Document Checklist", Description: "Interactive visa document checklist tool for immigrants", Version: "2.0.0", SettingKey: "checklist.enabled", SettingsCategory: "checklist", Category: "Tools"},
	{Key: "assessment", Name: "Japa Readiness Assessment", Description: "AI-assisted readiness assessment with personalized roadmap", Version: "2.0.0", SettingKey: "assessment.enabled", SettingsCategory: "assessment", Category: "Tools"},
	{Key: "col_calculator", Name: "Cost of Living Calculator", Description: "Compare cost of living across destination cities", Version: "2.0.0", SettingKey: "col_calculator.enabled", SettingsCategory: "col_calculator", Category: "Tools"},
	{Key: "scholarship_finder", Name: "Scholarship Finder", Description: "Search and track scholarships with deadline reminders", Version: "2.0.0", SettingKey: "scholarship_finder.enabled", SettingsCategory: "scholarship_finder", Category: "Tools"},
	{Key: "remittance", Name: "Remittance Rate Tracker", Description: "Track and compare remittance exchange rates over time", Version: "2.0.0", SettingKey: "remittance.enabled", SettingsCategory: "remittance", Category: "Tools"},
	{Key: "referral", Name: "Referral System", Description: "User referral program with credit rewards and attribution tracking", Version: "2.0.0", SettingKey: "referral.enabled", SettingsCategory: "referral", Category: "Commerce"},
	{Key: "ads", Name: "Self-Serve Advertising", Description: "Self-serve ad campaign creation and management platform", Version: "2.0.0", SettingKey: "ads.self_serve_enabled", SettingsCategory: "ads", Category: "Commerce"},
	{Key: "newsletter", Name: "Newsletter & Digest", Description: "Email newsletter subscriber management with weekly digests", Version: "2.0.0", SettingKey: "newsletter.enabled", SettingsCategory: "newsletter", Category: "Content"},
	{Key: "jobs", Name: "Jobs Board", Description: "Job listings board with featured placement and credit-based promotion", Version: "2.0.0", SettingKey: "jobs.enabled", SettingsCategory: "jobs", Category: "Commerce"},
	{Key: "gamification", Name: "Gamification", Description: "Badges, challenges, and leaderboards for user engagement", Version: "2.0.0", SettingKey: "gamification.badges_enabled", SettingsCategory: "gamification", Category: "Engagement"},
	{Key: "travel", Name: "Travel Search", Description: "Travel search and booking integration for immigration journeys", Version: "2.0.0", SettingKey: "travel.enabled", SettingsCategory: "travel", Category: "Tools"},
	{Key: "push", Name: "Mobile Push Notifications", Description: "FCM and APNs push notification delivery for mobile apps", Version: "2.0.0", SettingKey: "push.enabled", SettingsCategory: "push", Category: "Integrations"},
	{Key: "geo_personalization", Name: "Geo Personalization", Description: "Geo-aware feed, search, and discovery ranking based on user location", Version: "2.0.0", SettingKey: "geo_personalization.enabled", SettingsCategory: "geo_personalization", Category: "Integrations"},

	// V2 functional tools (§§90–97, §§99–101) — geo-aware tool pattern
	// (ADR-088). Each tool lives on the Tools web surface (§78, ADR-086)
	// and is toggled via its own catalogued system_settings key. These
	// mirror the apps/tools registry (apps/tools/src/lib/tools/registry.ts)
	// so the /extensions Tools tab is the canonical on/off switch for every
	// tool on the surface.
	{Key: "japabot", Name: "JapaBot", Description: "RAG-grounded AI japa guide chatbot on the Tools surface", Version: "2.0.0", SettingKey: "ai.japabot_enabled", SettingsCategory: "japabot", Category: "AI"},
	{Key: "visa_routes", Name: "Visa Route Comparison", Description: "Compare visa routes side by side — eligibility, fees, path to PR, and renewal terms", Version: "2.0.0", SettingKey: "visa_routes.enabled", SettingsCategory: "visa_routes", Category: "Tools"},
	{Key: "appointment_alert", Name: "Embassy Appointment Alerter", Description: "Subscribe to embassy appointment availability alerts and get notified when slots open", Version: "2.0.0", SettingKey: "appointment_alert.enabled", SettingsCategory: "appointment_alert", Category: "Tools"},
	{Key: "arrival_checklist", Name: "Arrival Checklist", Description: "Phased post-arrival task checklist — registration, banking, and settling-in steps", Version: "2.0.0", SettingKey: "arrival_checklist.enabled", SettingsCategory: "arrival_checklist", Category: "Tools"},
	{Key: "visa_timeline", Name: "Visa Processing Time Tracker", Description: "Visa processing time statistics and trends — average, median, and percentile wait times", Version: "2.0.0", SettingKey: "visa_timeline.enabled", SettingsCategory: "visa_timeline", Category: "Tools"},
	{Key: "credential_checker", Name: "Professional Credential Checker", Description: "Credential recognition pathways — regulatory body, timeline, cost, and steps", Version: "2.0.0", SettingKey: "credential_checker.enabled", SettingsCategory: "credential_checker", Category: "Tools"},
	{Key: "cv_formatter", Name: "CV Formatter", Description: "Destination-specific CV standards and AI-assisted formatting", Version: "2.0.0", SettingKey: "cv_formatter.enabled", SettingsCategory: "cv_formatter", Category: "Tools"},
	{Key: "housing_guide", Name: "Housing Guide", Description: "Destination-specific housing guides — platforms, landlord requirements, rent ranges, and scam categories", Version: "2.0.0", SettingKey: "housing_guide.enabled", SettingsCategory: "housing_guide", Category: "Tools"},
	{Key: "attestation_guide", Name: "Attestation Guide", Description: "Step-by-step document attestation pathways — apostille requirements, steps, and fees", Version: "2.0.0", SettingKey: "attestation_guide.enabled", SettingsCategory: "attestation_guide", Category: "Tools"},
	{Key: "language_test_hub", Name: "Language Test Hub", Description: "Required language tests for your visa — minimum scores, prep resources, and community score stories", Version: "2.0.0", SettingKey: "language_test_hub.enabled", SettingsCategory: "language_test_hub", Category: "Tools"},
	{Key: "tax_calculator", Name: "Tax Calculator", Description: "Destination-specific tax guides — residency triggers, worldwide income rules, and double taxation treaties", Version: "2.0.0", SettingKey: "tax_calculator.enabled", SettingsCategory: "tax_calculator", Category: "Tools"},
	{Key: "community_locator", Name: "Diaspora Community Locator", Description: "Find diaspora organisations near your destination — cultural groups, religious communities, and social clubs", Version: "2.0.0", SettingKey: "community_locator.enabled", SettingsCategory: "community_locator", Category: "Tools"},
}

func (h *Handlers) ListExtensions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	result := make([]extensionEntry, 0, len(knownExtensions))
	for _, ext := range knownExtensions {
		settingKey := ext.SettingKey
		if settingKey == "" {
			settingKey = "extensions." + ext.Key + ".enabled"
		}
		// Resolve the default enabled state from the settings catalog's
		// DefaultValue (the authoritative source, per Rule 3). Falls back to
		// the Version-based heuristic for legacy entries not in the catalog:
		// V2 tools default to enabled, legacy extensions default to disabled.
		defaultEnabled := ext.Version == "2.0.0"
		if meta, ok := settingsCatalog[settingKey]; ok && meta.Type == "bool" {
			defaultEnabled = meta.DefaultValue == "true"
		}
		enabled := defaultEnabled
		if h.Settings != nil {
			enabled = h.Settings.GetBoolOr(r.Context(), tenantID, settingKey, defaultEnabled)
		}
		ext.Enabled = enabled
		result = append(result, ext)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *Handlers) UpdateExtension(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "Extension key is required")
		return
	}
	ext := findExtension(key)
	if ext == nil {
		writeError(w, http.StatusNotFound, "extension_not_found", "Unknown extension or tool")
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	var input struct {
		Enabled *bool   `json:"enabled"`
		Version *string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Enabled != nil {
		settingKey := extensionSettingKey(ext)
		val := "false"
		if *input.Enabled {
			val = "true"
		}
		if _, err := h.DB.Writer.Exec(r.Context(),
			`INSERT INTO system_settings (tenant_id, key, value, updated_at) VALUES ($1, $2, $3, NOW()) ON CONFLICT (tenant_id, key) DO UPDATE SET value = $3, updated_at = NOW()`,
			tenantID, settingKey, val); err != nil {
			slog.Error("control: update extension", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update extension")
			return
		}
		if h.Settings != nil {
			h.Settings.InvalidateSettings(r.Context(), tenantID)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "status": "updated"}})
}

func (h *Handlers) EnableExtension(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "Extension key is required")
		return
	}
	ext := findExtension(key)
	if ext == nil {
		writeError(w, http.StatusNotFound, "extension_not_found", "Unknown extension or tool")
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	settingKey := extensionSettingKey(ext)
	if _, err := h.DB.Writer.Exec(r.Context(),
		`INSERT INTO system_settings (tenant_id, key, value, updated_at) VALUES ($1, $2, 'true', NOW()) ON CONFLICT (tenant_id, key) DO UPDATE SET value = 'true', updated_at = NOW()`,
		tenantID, settingKey); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to enable extension")
		return
	}
	if h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context(), tenantID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "status": "enabled"}})
}

func (h *Handlers) DisableExtension(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "Extension key is required")
		return
	}
	ext := findExtension(key)
	if ext == nil {
		writeError(w, http.StatusNotFound, "extension_not_found", "Unknown extension or tool")
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	settingKey := extensionSettingKey(ext)
	if _, err := h.DB.Writer.Exec(r.Context(),
		`INSERT INTO system_settings (tenant_id, key, value, updated_at) VALUES ($1, $2, 'false', NOW()) ON CONFLICT (tenant_id, key) DO UPDATE SET value = 'false', updated_at = NOW()`,
		tenantID, settingKey); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to disable extension")
		return
	}
	if h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context(), tenantID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "status": "disabled"}})
}

// findExtension returns the knownExtensions entry for the given key, or nil.
func findExtension(key string) *extensionEntry {
	for i := range knownExtensions {
		if knownExtensions[i].Key == key {
			return &knownExtensions[i]
		}
	}
	return nil
}

// extensionSettingKey returns the system_settings key that controls the
// enabled/disabled state for the given extension entry.
func extensionSettingKey(ext *extensionEntry) string {
	if ext.SettingKey != "" {
		return ext.SettingKey
	}
	return "extensions." + ext.Key + ".enabled"
}

// ---------------------------------------------------------------------------
// Backup Configs Handlers (stored in Valkey under "jc:backup_configs:")
// ---------------------------------------------------------------------------

type backupConfigEntry struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Schedule      string   `json:"schedule"`
	StorageType   string   `json:"storage_type"`
	Destination   string   `json:"destination"`
	WhatToBackup  []string `json:"what_to_backup"`
	RetentionDays int      `json:"retention_days"`
	Enabled       bool     `json:"enabled"`
	LastRunAt     string   `json:"last_run_at,omitempty"`
	CreatedAt     string   `json:"created_at"`
}

// supportedBackupTargets defines what can be included in a backup.
var supportedBackupTargets = []map[string]string{
	{"key": "database", "label": "Database", "description": "PostgreSQL full database dump"},
	{"key": "media", "label": "Media & Uploads", "description": "User-uploaded files, images, and attachments"},
	{"key": "valkey", "label": "Valkey (Redis)", "description": "Cached data, sessions, and queues"},
	{"key": "search_indexes", "label": "Search Indexes", "description": "Meilisearch index snapshots"},
	{"key": "logs", "label": "Application Logs", "description": "Structured application log archives"},
	{"key": "configs", "label": "Configuration", "description": "System settings and environment configuration"},
}

func backupConfigsKey(ctx context.Context) string {
	return valkey.MustKey("backup_configs:%s", tenant.IDFromContext(ctx).String())
}

func (h *Handlers) ListBackupConfigs(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}
	raw, err := h.Valkey.Get(r.Context(), backupConfigsKey(r.Context())).Result()
	if err != nil || raw == "" {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}
	var configs []backupConfigEntry
	if json.Unmarshal([]byte(raw), &configs) != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": configs})
}

// ListBackupTargets handles GET /control/backup-targets — returns the list of
// components that can be included in a backup.
func (h *Handlers) ListBackupTargets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": supportedBackupTargets})
}

func (h *Handlers) CreateBackupConfig(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		writeError(w, http.StatusServiceUnavailable, "no_valkey", "Valkey not available")
		return
	}
	var input struct {
		Name          string   `json:"name"`
		Schedule      string   `json:"schedule"`
		StorageType   string   `json:"storage_type"`
		Destination   string   `json:"destination"`
		WhatToBackup  []string `json:"what_to_backup"`
		RetentionDays int      `json:"retention_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Name == "" || input.Schedule == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "name and schedule are required")
		return
	}
	// Validate backup targets.
	validTargets := make(map[string]bool)
	for _, t := range supportedBackupTargets {
		validTargets[t["key"]] = true
	}
	for _, t := range input.WhatToBackup {
		if !validTargets[t] {
			writeError(w, http.StatusBadRequest, "invalid_target", "Unknown backup target: "+t)
			return
		}
	}
	// Default to database if nothing selected.
	if len(input.WhatToBackup) == 0 {
		input.WhatToBackup = []string{"database"}
	}
	configs := h.loadBackupConfigs(r)
	entry := backupConfigEntry{
		ID:            fmt.Sprintf("backup_%d", time.Now().UnixNano()),
		Name:          input.Name,
		Schedule:      input.Schedule,
		StorageType:   input.StorageType,
		Destination:   input.Destination,
		WhatToBackup:  input.WhatToBackup,
		RetentionDays: input.RetentionDays,
		Enabled:       true,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	configs = append(configs, entry)
	if err := h.saveBackupConfigs(r, configs); err != nil {
		writeError(w, http.StatusInternalServerError, "save_error", "Failed to save backup config")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": entry})
}

func (h *Handlers) UpdateBackupConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "id is required")
		return
	}
	var input struct {
		Name          *string  `json:"name"`
		Schedule      *string  `json:"schedule"`
		Destination   *string  `json:"destination"`
		WhatToBackup  []string `json:"what_to_backup"`
		RetentionDays *int     `json:"retention_days"`
		Enabled       *bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	configs := h.loadBackupConfigs(r)
	for i := range configs {
		if configs[i].ID == id {
			if input.Name != nil {
				configs[i].Name = *input.Name
			}
			if input.Schedule != nil {
				configs[i].Schedule = *input.Schedule
			}
			if input.Destination != nil {
				configs[i].Destination = *input.Destination
			}
			if input.WhatToBackup != nil {
				configs[i].WhatToBackup = input.WhatToBackup
			}
			if input.RetentionDays != nil {
				configs[i].RetentionDays = *input.RetentionDays
			}
			if input.Enabled != nil {
				configs[i].Enabled = *input.Enabled
			}
			if err := h.saveBackupConfigs(r, configs); err != nil {
				writeError(w, http.StatusInternalServerError, "save_error", "Failed to update backup config")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": configs[i]})
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "Backup config not found")
}

func (h *Handlers) DeleteBackupConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "id is required")
		return
	}
	configs := h.loadBackupConfigs(r)
	for i, c := range configs {
		if c.ID == id {
			configs = append(configs[:i], configs[i+1:]...)
			if err := h.saveBackupConfigs(r, configs); err != nil {
				writeError(w, http.StatusInternalServerError, "save_error", "Failed to delete backup config")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "Backup config not found")
}

func (h *Handlers) TriggerBackup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "id is required")
		return
	}
	if h.Client == nil {
		writeError(w, http.StatusServiceUnavailable, "queue_unavailable", "Task queue not available")
		return
	}
	configs := h.loadBackupConfigs(r)
	for i := range configs {
		if configs[i].ID == id {
			configs[i].LastRunAt = time.Now().UTC().Format(time.RFC3339)
			if err := h.saveBackupConfigs(r, configs); err != nil {
				slog.Error("control: trigger backup update last_run", "error", err)
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": id, "status": "triggered"}})
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "Backup config not found")
}

func (h *Handlers) loadBackupConfigs(r *http.Request) []backupConfigEntry {
	if h.Valkey == nil {
		return []backupConfigEntry{}
	}
	raw, err := h.Valkey.Get(r.Context(), backupConfigsKey(r.Context())).Result()
	if err != nil || raw == "" {
		return []backupConfigEntry{}
	}
	var configs []backupConfigEntry
	if json.Unmarshal([]byte(raw), &configs) != nil {
		return []backupConfigEntry{}
	}
	return configs
}

func (h *Handlers) saveBackupConfigs(r *http.Request, configs []backupConfigEntry) error {
	if h.Valkey == nil {
		return fmt.Errorf("valkey not available")
	}
	data, err := json.Marshal(configs)
	if err != nil {
		return err
	}
	return h.Valkey.Set(r.Context(), backupConfigsKey(r.Context()), string(data), 0).Err()
}
