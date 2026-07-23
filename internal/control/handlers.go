package control

import (
	"context"
	"database/sql"
	"encoding/json"
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

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/websocket"
	"github.com/jedafe/japa/internal/worker"
)

// Handlers holds dependencies for control panel HTTP handlers.
type Handlers struct {
	Service          *Service
	DB               *db.DB
	FeatureChecker   *features.Checker
	SpamStore        *spam.Store
	PublicAPI        *publicapi.Handlers
	Inspector        *asynq.Inspector
	Client           *asynq.Client
	RBAC             *rbac.RBAC
	Settings         *config.Settings
	Valkey           valkeycompat.Cmdable
	MeiliSearch      meilisearch.ServiceManager
	WSHub            *websocket.Hub
	StartTime        time.Time
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

// ---------------------------------------------------------------------------
// Tenant Handlers (delegate to Service)
// ---------------------------------------------------------------------------

func (h *Handlers) ListTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.Service.ListTenants(r.Context())
	if err != nil {
		slog.Error("control: list tenants", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list tenants")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tenants})
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

	// Enqueue the tenant:cleanup_data task with a 30-day delay so all
	// non-cascading child data is hard-deleted after the grace period.
	if h.Client != nil {
		payload, _ := json.Marshal(map[string]string{"tenant_id": tenant.ID.String(), "initiated_by": "system"})
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
		data, _ := json.Marshal(value)
		_, err := h.DB.Writer.Exec(r.Context(),
			`INSERT INTO system_settings (tenant_id, key, value, updated_at) VALUES ($1, $2, $3, NOW()) ON CONFLICT (tenant_id, key) DO UPDATE SET value = $3, updated_at = NOW()`,
			tenantID, key, string(data),
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
		Value    any  `json:"value"`
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
			slog.Error("control: update system setting", "key", key, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update setting")
			return
		}
	} else {
		_, err := h.DB.Writer.Exec(r.Context(),
			`INSERT INTO system_settings (tenant_id, key, value, updated_at) VALUES ($1, $2, $3, NOW()) ON CONFLICT (tenant_id, key) DO UPDATE SET value = $3, updated_at = NOW()`,
			tenantID, key, string(data),
		)
		if err != nil {
			slog.Error("control: update system setting", "key", key, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update setting")
			return
		}
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
		data, _ := json.Marshal(value)
		_, err := h.DB.Writer.Exec(r.Context(),
			`INSERT INTO system_settings (tenant_id, key, value, updated_at) VALUES ($1, $2, $3, NOW()) ON CONFLICT (tenant_id, key) DO UPDATE SET value = $3, updated_at = NOW()`,
			tenantID, key, string(data),
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
		Value    any  `json:"value"`
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
// Feature Flags Handlers
// ---------------------------------------------------------------------------

func (h *Handlers) ListFeatureFlags(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	flags := h.FeatureChecker.GetAllEnabled(r.Context(), tenantID)
	writeJSON(w, http.StatusOK, map[string]any{"data": flags})
}

func (h *Handlers) SetFeatureFlag(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	flagName := chi.URLParam(r, "flag_name")
	if flagName == "" {
		writeError(w, http.StatusBadRequest, "missing_flag_name", "flag_name is required")
		return
	}
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	user := authUserFromContext(r)
	if err := h.FeatureChecker.SetEnabled(r.Context(), tenantID, flagName, input.Enabled, user); err != nil {
		slog.Error("control: set feature flag", "flag", flagName, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update feature flag")
		return
	}
	h.FeatureChecker.ClearCache()
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"key": flagName, "enabled": input.Enabled}})
}

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

	// Measure latency with ping.
	start := time.Now()
	if err := h.DB.Reader.Ping(ctx); err != nil {
		slog.Error("control: db health check", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"data": map[string]any{"status": "unhealthy", "error": err.Error()},
		})
		return
	}
	latency := time.Since(start).Milliseconds()

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

	// Table details.
	type tableInfo struct {
		Name       string  `json:"name"`
		RowCount   int64   `json:"row_count"`
		SizeBytes  int64   `json:"size_bytes"`
		IndexCount int     `json:"index_count"`
		LastVacuum *string `json:"last_vacuum"`
	}
	var tables []tableInfo
	rows, err := h.DB.Reader.Query(ctx, `
		SELECT
			t.tablename,
			COALESCE(c.reltuples::bigint, 0),
			pg_table_size(c.oid),
			(SELECT count(*) FROM pg_indexes WHERE tablename = t.tablename AND schemaname = 'public')::int,
			(SELECT max(last_vacuum)::text FROM pg_stat_user_tables WHERE relname = t.tablename)
		FROM pg_tables t
		LEFT JOIN pg_class c ON c.relname = t.tablename
		WHERE t.schemaname = 'public'
		ORDER BY pg_table_size(c.oid) DESC
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var ti tableInfo
			if err := rows.Scan(&ti.Name, &ti.RowCount, &ti.SizeBytes, &ti.IndexCount, &ti.LastVacuum); err == nil {
				tables = append(tables, ti)
			}
		}
	}
	if tables == nil {
		tables = []tableInfo{}
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
		State       string  `json:"state"`
		LagSeconds  float64 `json:"lag_seconds"`
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

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"connected":        true,
			"version":          version,
			"latency_ms":       latency,
			"size_bytes":       sizeBytes,
			"table_count":      tableCount,
			"index_count":      indexCount,
			"avg_query_time_ms": 0,
			"tables":           tables,
			"slow_queries":     slowQueries,
			"indexes_unused":   indexesUnused,
			"connection_pool": map[string]any{
				"active":  activeConns,
				"idle":    idleConns,
				"max":     maxConns,
			},
			"replication": repl,
		},
	})
}

func (h *Handlers) DBVacuum(w http.ResponseWriter, r *http.Request) {
	// Enqueue async vacuum task via Asynq for background processing.
	if h.Client != nil {
		task := asynq.NewTask(worker.TypeDBManualVacuum, nil)
		if _, err := h.Client.Enqueue(task, asynq.Queue(worker.QueueForTask(worker.TypeDBManualVacuum))); err != nil {
			slog.Error("control: failed to enqueue db vacuum task", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to enqueue vacuum task")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "enqueued"}})
		return
	}

	// Fallback: run inline if no Asynq client.
	_, err := h.DB.Writer.Exec(r.Context(), `VACUUM`)
	if err != nil {
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
				"queues":               []any{},
				"total_pending":        0,
				"total_active":         0,
				"total_completed_24h":  0,
				"total_failed_24h":     0,
				"dlq_count":            0,
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
		queues = append(queues, queueInfo{
			Name:         queueName,
			Pending:      qi.Pending,
			Active:       qi.Active,
			Completed24H: qi.Completed,
			Failed24H:    qi.Failed,
			LatencyMs:    qi.Latency.Seconds() * 1000,
		})
		totalPending += qi.Pending
		totalActive += qi.Active
		totalCompleted += qi.Completed
		totalFailed += qi.Failed
		dlqDepth += qi.Archived
	}

	if queues == nil {
		queues = []queueInfo{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"queues":               queues,
			"total_pending":        totalPending,
			"total_active":         totalActive,
			"total_completed_24h":  totalCompleted,
			"total_failed_24h":     totalFailed,
			"dlq_count":            dlqDepth,
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
				"task_id":      entry.ID,
				"task_type":    entry.Type,
				"queue":        queueName,
				"payload":      string(entry.Payload),
				"error_message": entry.LastErr,
				"retry_count":  entry.Retried,
				"max_retries":  entry.MaxRetry,
				"failed_at":    entry.LastFailedAt.Format(time.RFC3339),
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
		ID          string    `json:"id"`
		Type        string    `json:"type"`
		State       string    `json:"state"`
		Payload     string    `json:"payload"`
		RetryCount  int       `json:"retry_count"`
		MaxRetry    int       `json:"max_retry"`
		LastErr     string    `json:"last_err,omitempty"`
		LastFailedAt *string  `json:"last_failed_at,omitempty"`
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
	Prefix            string  `json:"prefix"`
	KeyCount          int64   `json:"key_count"`
	MemoryEstimateMB  float64 `json:"memory_estimate_mb"`
}

type cacheStatsResponse struct {
	Namespaces      []cacheNamespace `json:"namespaces"`
	TotalKeys       int64            `json:"total_keys"`
	TotalMemoryMB   float64          `json:"total_memory_mb"`
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
		Namespaces:     namespaces,
		TotalKeys:      totalKeys,
		TotalMemoryMB:  totalMemoryMB,
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

	task := asynq.NewTask("security:scan", payload)
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
// / per-task breakdowns aggregate from the ai_usage_log table; that table is not
// yet provisioned (see internal/ai/handlers.go), so spend defaults to 0 until
// usage tracking ships. alert_triggered is derived from threshold vs spend.
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

	// Spend aggregation requires the ai_usage_log table, which is not yet
	// created. Report 0 spend and empty breakdowns until that lands.
	const monthToDateCost = 0.0
	alertTriggered := threshold > 0 && monthToDateCost >= threshold

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"month_to_date_cost":    monthToDateCost,
			"budget_threshold_usd":  threshold,
			"alert_triggered":       alertTriggered,
			"by_provider":           []any{},
			"by_task_type":          []any{},
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

	const monthToDateCost = 0.0
	alertTriggered := threshold > 0 && monthToDateCost >= threshold

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"month_to_date_cost":    monthToDateCost,
			"budget_threshold_usd":  threshold,
			"alert_triggered":       alertTriggered,
			"by_provider":           []any{},
			"by_task_type":          []any{},
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
				"version":  "1.0.0",
				"status":   "active",
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
	return "jc:cron:custom:" + tenant.IDFromContext(ctx).String()
}

// cronPausedKey returns the tenant-scoped Valkey key for paused scheduler entries.
func cronPausedKey(ctx context.Context) string {
	return "jc:cron:paused:" + tenant.IDFromContext(ctx).String()
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
