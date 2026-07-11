package control

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/tenant"
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
	RBAC             *rbac.RBAC
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

// SetRBAC injects the RBAC resolver for cache operations.
func (h *Handlers) SetRBAC(r *rbac.RBAC) {
	h.RBAC = r
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
	if err := h.Service.DeleteTenant(r.Context(), id); err != nil {
		slog.Error("control: delete tenant", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete tenant")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
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

	type setting struct {
		ID        pgtype.UUID `json:"id"`
		Key       string      `json:"key"`
		Value     string      `json:"value"`
		IsPublic  bool        `json:"is_public"`
		CreatedAt time.Time   `json:"created_at"`
		UpdatedAt time.Time   `json:"updated_at"`
	}
	var settings []setting
	for rows.Next() {
		var s setting
		if err := rows.Scan(&s.ID, &s.Key, &s.Value, &s.IsPublic, &s.CreatedAt, &s.UpdatedAt); err != nil {
			slog.Error("control: scan setting", "error", err)
			continue
		}
		settings = append(settings, s)
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
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Setting not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "status": "deleted"}})
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
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"status":     "ok",
			"uptime_sec": int(time.Since(h.StartTime).Seconds()),
			"go_version": runtime.Version(),
			"goroutines": runtime.NumGoroutine(),
		},
	})
}

func (h *Handlers) DBHealthCheck(w http.ResponseWriter, r *http.Request) {
	if err := h.DB.Reader.Ping(r.Context()); err != nil {
		slog.Error("control: db health check", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"data": map[string]any{"status": "unhealthy", "error": err.Error()},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"status": "ok"},
	})
}

func (h *Handlers) DBVacuum(w http.ResponseWriter, r *http.Request) {
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
				"message": "Queue stats require Asynq inspector integration",
			},
		})
		return
	}
	worker.HandleQueueStats(h.Inspector)(w, r)
}

func (h *Handlers) ListDLQ(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	var tasks []map[string]any

	for _, queueName := range []string{"critical", "default", "low"} {
		entries, err := h.Inspector.ListRetryTasks(queueName)
		if err != nil {
			slog.Warn("dlq: list tasks failed", "queue", queueName, "error", err)
			continue
		}
		for _, entry := range entries {
			tasks = append(tasks, map[string]any{
				"id":          entry.ID,
				"type":        entry.Type,
				"queue":       queueName,
				"payload":     string(entry.Payload),
				"error":       entry.LastErr,
				"retries":     entry.Retried,
				"max_retries": entry.MaxRetry,
				"last_failed": entry.LastFailedAt.Format("2006-01-02T15:04:05Z"),
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tasks})
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
		entries, err := h.Inspector.ListRetryTasks(queueName)
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
// Cache Management Handlers (stubs — require Valkey client)
// ---------------------------------------------------------------------------

func (h *Handlers) CacheStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"message": "Cache stats require Valkey INFO integration",
		},
	})
}

func (h *Handlers) FlushCache(w http.ResponseWriter, r *http.Request) {
	if h.RBAC == nil {
		writeError(w, http.StatusServiceUnavailable, "rbac_unavailable", "RBAC not configured")
		return
	}
	h.RBAC.InvalidateAllCache(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"message": "Permission cache flushed"},
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

func (h *Handlers) GetAIBudget(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"message": "AI budget endpoint requires implementation",
		},
	})
}

func (h *Handlers) UpdateAIBudget(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]any{
		"error": map[string]string{"code": "not_implemented", "message": "AI budget update requires implementation"},
	})
}

// ---------------------------------------------------------------------------
// Audit Log Handlers
// ---------------------------------------------------------------------------

func (h *Handlers) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	type auditLog struct {
		ID        pgtype.UUID `json:"id"`
		UserID    pgtype.UUID `json:"user_id"`
		Action    string      `json:"action"`
		Resource  string      `json:"resource"`
		CreatedAt time.Time   `json:"created_at"`
	}
	rows, err := h.DB.Reader.Query(r.Context(),
		`SELECT id, user_id, action, resource_type, created_at FROM audit_logs ORDER BY created_at DESC LIMIT 100`,
	)
	if err != nil {
		slog.Error("control: list audit logs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list audit logs")
		return
	}
	defer rows.Close()

	var logs []auditLog
	for rows.Next() {
		var l auditLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.Resource, &l.CreatedAt); err != nil {
			slog.Error("control: scan audit log", "error", err)
			continue
		}
		logs = append(logs, l)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": logs})
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
	h.PublicAPI.ListKeys(w, r)
}

func (h *Handlers) UpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.RotateKey(w, r)
}

func (h *Handlers) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.RevokeKey(w, r)
}

func (h *Handlers) GetAPIKeyAnalytics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"message": "API key analytics requires implementation",
		},
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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
