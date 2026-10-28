package control

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/cdn"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/honcho"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/timeparse"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/websocket"
	"github.com/jedafe/japa/internal/worker"
)

// encodeSettingValue returns a JSON byte slice suitable for the
// system_settings.value jsonb column. If the supplied value is a string that
// itself contains a JSON array or object, the raw JSON is stored directly
// instead of double-encoding it as a JSON string.
func encodeSettingValue(value any) []byte {
	if s, ok := value.(string); ok {
		trimmed := strings.TrimSpace(s)
		if (strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) ||
			(strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) {
			if json.Valid([]byte(trimmed)) {
				return []byte(trimmed)
			}
		}
	}

	data, _ := json.Marshal(value)

	return data
}

// HonchoHealthChecker checks Honcho service health for the Control Panel
// application health group (ADR-027, Section 21 line 104). The honcho.Client
// satisfies this interface. Health returns a report carrying connectivity
// plus any sync-lag / store-size metrics the server exposes.
type HonchoHealthChecker interface {
	Health(ctx context.Context) (*honcho.HealthReport, error)
}

// SearchHealthChecker checks the search engine for the Control Panel
// application health group. The search.Engine interface satisfies this.
// Name supplies the display label (e.g. "Meilisearch") so the health view
// stays accurate if the engine changes.
type SearchHealthChecker interface {
	Health(ctx context.Context) error
	Name() string
}

// Handlers holds dependencies for control panel HTTP handlers.
type Handlers struct {
	DB                *db.DB
	FeatureChecker    *features.Checker
	SpamStore         *spam.Store
	PublicAPI         *publicapi.Handlers
	Inspector         *asynq.Inspector
	Client            *asynq.Client
	RBAC              *rbac.RBAC
	Settings          *config.Settings
	Valkey            valkeycompat.Cmdable
	SearchEngine      SearchHealthChecker
	WSHub             *websocket.Hub
	Honcho            HonchoHealthChecker
	AIStore           *ai.Store
	ProviderStore     *ai.ProviderStore
	ByokStore         *ai.ByokProviderStore
	AIDispatcher      *ai.Dispatcher
	StartTime         time.Time
	DeploymentVersion string
	// onFlagEnabled fires when a flag transitions off→on via the extension
	// write endpoints (enable/disable, PATCH) — mirrors the activation hook
	// on features.Handlers so enabling e.g. wiki.enabled from /extensions
	// seeds stub pages exactly as the admin flag endpoints do. Optional.
	onFlagEnabled func(ctx context.Context, key string)
}

// SetOnFlagEnabled registers the off→on activation hook. Wired in server.go
// to the same callback as features.Handlers.SetOnFlagEnabled.
func (h *Handlers) SetOnFlagEnabled(fn func(ctx context.Context, key string)) {
	h.onFlagEnabled = fn
}

// fireFlagEnabled runs the activation hook when a write transitioned the
// flag from off to on. Fire-and-forget on a detached context (Rule 8).
func (h *Handlers) fireFlagEnabled(ctx context.Context, key string, wasEnabled, nowEnabled bool) {
	if h.onFlagEnabled == nil || wasEnabled || !nowEnabled {
		return
	}

	go h.onFlagEnabled(context.WithoutCancel(ctx), key)
}

// NewHandlers creates a new Handlers.
func NewHandlers(database *db.DB, featureChecker *features.Checker, spamStore *spam.Store, publicAPI *publicapi.Handlers) *Handlers {
	return &Handlers{
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

// SetSearchEngine injects the search engine for health checks.
func (h *Handlers) SetSearchEngine(s SearchHealthChecker) {
	h.SearchEngine = s
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

// SetByokProviderStore injects the BYOK allowlist store for the
// /control/ai-byok-providers endpoints.
func (h *Handlers) SetByokProviderStore(s *ai.ByokProviderStore) {
	h.ByokStore = s
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

// SetDeploymentVersion sets the deployment version override. When empty,
// ListDeployments falls back to BuildVersion.
func (h *Handlers) SetDeploymentVersion(version string) {
	h.DeploymentVersion = version
}

// ---------------------------------------------------------------------------
// System Settings Handlers
// ---------------------------------------------------------------------------

// ListSystemSettings is an exported method.
func (h *Handlers) ListSystemSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Reader.Query(r.Context(), `SELECT id, key, value, is_public, created_at, updated_at FROM system_settings  ORDER BY key`)
	if err != nil {
		slog.Error("control: list system settings", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list settings")

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

	// Currency-valued settings (pricing.currency, payment.currency) take their
	// options from the managed currencies table — the catalog's static option
	// list predates operator-managed currencies (§58, Rule 16).
	currencyOpts := h.currencySettingOptions(r.Context())

	for rows.Next() {
		var s enrichedSetting
		if err := rows.Scan(&s.ID, &s.Key, &s.Value, &s.IsPublic, &s.CreatedAt, &s.UpdatedAt); err != nil {
			slog.Error("control: scan setting", "error", err)
			continue
		}
		// Enrich with authoritative metadata from the settings catalog.
		meta := getSettingMeta(s.Key, s.Value)
		// Reachability toggles migrated to feature_flags are hidden from the
		// editable list — manage them via /extensions (Task 13). The check
		// consults deprecatedReachabilitySettings directly so stored rows
		// for deprecated keys WITHOUT a catalog entry (service_packages.
		// enabled, sop_generator.enabled, …) are excluded too.
		if isDeprecatedSettingKey(s.Key) {
			continue
		}

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

		if opts, ok := currencyOpts[s.Key]; ok {
			s.Options = opts
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
		// Skip deprecated reachability toggles (feature_flags is authoritative).
		if isDeprecatedSettingKey(key) {
			continue
		}

		var ns string
		if idx := strings.IndexByte(key, '.'); idx >= 0 {
			ns = key[:idx]
		}

		var opts []any
		if meta.Options != "" {
			if err := json.Unmarshal([]byte(meta.Options), &opts); err == nil {
				// opts parsed successfully; it will be assigned to Options below.
			}
		}

		if cOpts, ok := currencyOpts[key]; ok {
			opts = cOpts
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": settings})
}

// currencySettingKeys are the system_settings keys whose value is an ISO 4217
// currency code drawn from the managed currencies table (§58).
var currencySettingKeys = map[string]bool{
	"pricing.currency":        true,
	"payment.currency":        true,
	"wallet.default_currency": true,
}

// currencySettingOptions builds {value:"CODE", label:"Name (CODE)"} options
// from the active currencies table for currency-valued settings. Returns nil
// when the table can't be read — callers keep the catalog's static options.
func (h *Handlers) currencySettingOptions(ctx context.Context) map[string][]any {
	currencies, err := gen.New(h.DB.Reader).ListActiveCurrencies(ctx)
	if err != nil || len(currencies) == 0 {
		return nil
	}

	opts := make([]any, 0, len(currencies))

	for _, c := range currencies {
		label := c.Name
		if label == "" {
			label = c.Code
		}

		opts = append(opts, map[string]any{"value": c.Code, "label": label + " (" + c.Code + ")"})
	}

	out := make(map[string][]any, len(currencySettingKeys))
	for k := range currencySettingKeys {
		out[k] = opts
	}

	return out
}

func (h *Handlers) UpdateSystemSettings(w http.ResponseWriter, r *http.Request) {
	var input map[string]any
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	for key, value := range input {
		// Validate the value against the catalog (type + min/max bounds).
		// The frontend sends values as strings; for non-string types we
		// validate the string representation before persisting.
		if s, ok := value.(string); ok {
			if verr := validateSettingValue(key, s); verr != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_setting_value", verr.Error())
				return
			}
		}
		// The system_settings.value column is jsonb, so every value must be
		// valid JSON. encodeSettingValue stores raw JSON when the frontend sends
		// a JSON array/object as a string (e.g. '["uuid"]'), avoiding
		// double-encoding.
		data := encodeSettingValue(value)
		// Consult the catalog for the is_public flag. On INSERT, set is_public
		// from the catalog so appearance/branding settings are exposed via
		// GET /site/settings/public. On CONFLICT (existing row), preserve the
		// current is_public value — an admin may have explicitly toggled it.
		isPublic := false
		if meta, ok := settingsCatalog[key]; ok {
			isPublic = meta.Public
		}

		_, err := h.DB.Writer.Exec(r.Context(), `INSERT INTO system_settings (key, value, is_public, updated_at) VALUES ($1, $2, $3, NOW()) ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = NOW()`, key, string(data), isPublic)
		if err != nil {
			slog.Error("control: update system setting", "key", key, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update setting: "+key)

			return
		}
	}

	if h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context())
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// UpdateSystemSetting is an exported method.
func (h *Handlers) UpdateSystemSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Setting key is required")
		return
	}

	var input struct {
		Value    any   `json:"value"`
		IsPublic *bool `json:"is_public"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	// Validate the value against the catalog (type + min/max bounds).
	if s, ok := input.Value.(string); ok {
		if verr := validateSettingValue(key, s); verr != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_setting_value", verr.Error())
			return
		}
	}
	// The value column is jsonb — encodeSettingValue stores raw JSON when the
	// value is a string that itself is a JSON array/object, preventing
	// double-encoding.
	data := encodeSettingValue(input.Value)
	// Resolve is_public: explicit request body value takes precedence; fall
	// back to the catalog default so appearance/branding settings are public.
	isPublic := false
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	} else if meta, ok := settingsCatalog[key]; ok {
		isPublic = meta.Public
	}

	_, err := h.DB.Writer.Exec(r.Context(), `INSERT INTO system_settings (key, value, is_public, updated_at) VALUES ($1, $2, $3, NOW()) ON CONFLICT (key) DO UPDATE SET value = $2, is_public = $3, updated_at = NOW()`, key, string(data), isPublic)
	if err != nil {
		slog.Error("control: update system setting", "key", key, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update setting")

		return
	}

	if h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context())
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "status": "updated"}})
}

// ResetSystemSetting is an exported method.
func (h *Handlers) ResetSystemSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Setting key is required")
		return
	}

	tag, err := h.DB.Writer.Exec(r.Context(), `DELETE FROM system_settings WHERE key = $1`, key)
	if err != nil {
		slog.Error("control: reset system setting", "key", key, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to reset setting")

		return
	}

	if h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context())
	}
	// Look up the default value from the catalog; if not found, the setting was
	// deleted but had no catalog entry — return an empty default.
	defaultValue := ""
	if meta, ok := settingsCatalog[key]; ok {
		defaultValue = meta.DefaultValue
	}

	if tag.RowsAffected() == 0 {
		// Setting didn't exist in DB (already at default). Return the default value.
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "value": defaultValue, "default_value": defaultValue}})
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "value": defaultValue, "default_value": defaultValue}})
}

// ---------------------------------------------------------------------------
// Edge CDN (ADR-134) — status + credential validation.
// ---------------------------------------------------------------------------

// CDNStatus reports the resolved edge CDN configuration and the outcome of
// the most recent purge so the Settings → Edge CDN card can show whether
// the integration is working. Credential fields are presence flags only —
// values are never returned.
func (h *Handlers) CDNStatus(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cdn.StatusSnapshot()})
}

// CDNTestPurge validates the configured provider credentials end-to-end by
// issuing a real purge for a sentinel URL (ADR-134). A URL purge exercises
// auth + zone/pull-zone resolution on every plan; tag purge is reported
// separately via status.tag_purge_ready since Cloudflare restricts it to
// Enterprise.
func (h *Handlers) CDNTestPurge(w http.ResponseWriter, r *http.Request) {
	if !cdn.Enabled() {
		errpkg.WriteError(w, http.StatusBadRequest, "cdn_disabled", "No edge CDN provider is configured")
		return
	}

	snap := cdn.StatusSnapshot()
	if !snap.CredentialsConfigured {
		errpkg.WriteError(w, http.StatusBadRequest, "cdn_not_configured", "Provider credentials are incomplete — save the required settings first")
		return
	}

	if snap.SiteURL == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "cdn_no_site_url", "JC_SITE_URL is unset — set the public site URL so URL purges can be issued")
		return
	}

	probe := snap.SiteURL + "/cdn-probe-" + time.Now().UTC().Format("20060102150405")
	if err := cdn.Purge(r.Context(), cdn.PurgeRequest{URLs: []string{probe}}); err != nil {
		errpkg.WriteError(w, http.StatusBadGateway, "cdn_purge_failed", "Purge request failed: "+err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok", "url": probe}})
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

// HealthCheck is an exported method.
func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type serviceStatus struct {
		Name      string `json:"name"`
		Status    string `json:"status"`
		LatencyMs int64  `json:"latency_ms"`
		Message   string `json:"message,omitempty"`
		Details   string `json:"details,omitempty"`
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

	if h.SearchEngine != nil {
		application = append(application, checkService(h.SearchEngine.Name(), func(ctx context.Context) error {
			return h.SearchEngine.Health(ctx)
		}))
	}

	// Honcho agent-memory service (ADR-027: Control Panel application health
	// group — Section 21 line 104 places honcho alongside go_api and
	// meilisearch in the application group, not infrastructure). The health
	// report carries sync-lag and store-size metrics when the server exposes
	// them (Section 21 "details?: string").
	if h.Honcho != nil {
		start := time.Now()
		report, err := h.Honcho.Health(ctx)
		latency := time.Since(start).Milliseconds()

		if err != nil {
			application = append(application, serviceStatus{
				Name: "Honcho", Status: "down", LatencyMs: latency, Message: err.Error(),
			})
		} else if !report.Healthy {
			application = append(application, serviceStatus{
				Name: "Honcho", Status: "down", LatencyMs: latency,
				Message: report.Details,
			})
		} else {
			application = append(application, serviceStatus{
				Name: "Honcho", Status: "healthy", LatencyMs: latency,
				Details: report.Details,
			})
		}
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
		errpkg.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
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
	// We report WAL lag in bytes from the primary's pg_stat_replication and
	// the replay timestamp from the replica. The timestamp-based "lag_seconds"
	// is unreliable during idle periods (it grows when no writes occur), so
	// the health judgment is based on WAL lag bytes, not seconds.
	type replicationInfo struct {
		State       string  `json:"state"`
		LagSeconds  float64 `json:"lag_seconds"`
		LagBytes    int64   `json:"lag_bytes"`
		LagPretty   string  `json:"lag_pretty"`
		Healthy     bool    `json:"healthy"`
		HealthyText string  `json:"healthy_text"`
	}

	var repl *replicationInfo

	var isRecovery bool

	var replLagSeconds float64

	err = h.DB.Reader.QueryRow(ctx, `
		SELECT
			pg_is_in_recovery(),
			COALESCE(EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()))::float, 0)
	`).Scan(&isRecovery, &replLagSeconds)
	if err == nil {
		state := "primary"
		if isRecovery {
			state = "standby"
		}

		// WAL lag from the primary's pg_stat_replication. This is the real
		// measure of how far behind the replica is.
		var lagBytes int64
		_ = h.DB.Writer.QueryRow(ctx, `
			SELECT COALESCE(pg_wal_lsn_diff(sent_lsn, replay_lsn), 0)
			FROM pg_stat_replication
			LIMIT 1
		`).Scan(&lagBytes)

		// Healthy if no replica is configured (single-node), or if WAL lag
		// is under 100 MB. Timestamp lag is only concerning when there is
		// actual WAL backlog.
		const unhealthyLagBytes = 100 * 1024 * 1024 // 100 MB
		healthy := lagBytes < unhealthyLagBytes
		healthyText := "Replica is caught up or within safe lag"

		if !healthy {
			healthyText = "Replica lag exceeds 100 MB WAL backlog"
		} else if lagBytes == 0 && !isRecovery {
			healthyText = "Single-node database (no replica configured)"
		} else if lagBytes == 0 {
			healthyText = "Replica fully caught up (0 bytes WAL lag)"
		}

		repl = &replicationInfo{
			State:       state,
			LagSeconds:  replLagSeconds,
			LagBytes:    lagBytes,
			LagPretty:   humanizeBytes(lagBytes),
			Healthy:     healthy,
			HealthyText: healthyText,
		}
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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

// DBVacuum is an exported method.
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
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_request_body", "Malformed JSON body")
				return
			}
		}
	}

	// Enqueue async vacuum task via Asynq for background processing.
	if h.Client != nil {
		payload.Version = 1 // current payload schema version

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to encode vacuum payload")
			return
		}

		task := asynq.NewTask(worker.TypeDBManualVacuum, payloadBytes)
		if _, err := h.Client.Enqueue(task, asynq.Queue(worker.QueueForTask(worker.TypeDBManualVacuum))); err != nil {
			slog.Error("control: failed to enqueue db vacuum task", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to enqueue vacuum task")

			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "enqueued"}})

		return
	}

	// Fallback: run inline if no Asynq client. Use a detached context so
	// the VACUUM is not cancelled by the request deadline (VACUUM on large
	// tables can exceed the 30s WriteTimeout).
	vacuumCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Minute)
	defer cancel()

	if err := h.DB.Vacuum(vacuumCtx, payload.TableName, payload.Full); err != nil {
		slog.Error("control: db vacuum", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to run VACUUM")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "completed"}})
}

// ---------------------------------------------------------------------------
// Queue Monitor Handlers (wired to Asynq inspector — BE-040)
// ---------------------------------------------------------------------------

// QueueStats returns Asynq queue statistics for the control-panel queue monitor (BE-040).
func (h *Handlers) QueueStats(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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

// existingQueues returns the set of queue names Asynq knows about
// (Inspector.Queues is the only public existence check — the *All* delete
// methods return an internal NotFound kind for never-created queues, not
// the exported ErrQueueNotFound). The bool reports whether the lookup
// succeeded; callers must ignore the map when it is false.
func existingQueues(inspector *asynq.Inspector) (map[string]bool, bool) {
	names, err := inspector.Queues()
	if err != nil {
		return nil, false
	}

	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}

	return set, true
}

// listAllTasks pages through an Inspector list method — the Inspector's
// default page size is 30, so a single call silently truncates larger
// state sets (e.g. dead-letter queues).
func listAllTasks(
	listFn func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error),
	queueName string,
) ([]*asynq.TaskInfo, error) {
	return listTasksUpTo(listFn, queueName, 0)
}

// listTasksUpTo pages through an Inspector list method like listAllTasks
// but stops once max entries have been collected (max <= 0 means no
// limit). Prevents a "recent tasks" preview from shipping thousands of
// rows when a state set is deep.
func listTasksUpTo(
	listFn func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error),
	queueName string,
	max int,
) ([]*asynq.TaskInfo, error) {
	const pageSize = 200

	var all []*asynq.TaskInfo

	for page := 1; ; page++ {
		entries, err := listFn(queueName, asynq.Page(page), asynq.PageSize(pageSize))
		if err != nil {
			return nil, err
		}

		all = append(all, entries...)

		if len(entries) < pageSize || (max > 0 && len(all) >= max) {
			if max > 0 && len(all) > max {
				all = all[:max]
			}
			return all, nil
		}
	}
}

// ListDLQ lists dead-letter queue tasks for the control-panel queue monitor.
func (h *Handlers) ListDLQ(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	tasks := make([]map[string]any, 0)

	for _, queueName := range []string{"critical", "default", "low"} {
		entries, err := listAllTasks(h.Inspector.ListArchivedTasks, queueName)
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tasks})
}

// maxQueueTaskPreview bounds how many tasks per state ListQueueTasks
// returns. The endpoint backs the "recent jobs" drill-down — a queue can
// hold tens of thousands of archived/completed tasks, and shipping them
// all produces a multi-MB response that hangs the modal. True per-state
// totals ride alongside in the counts map.
const maxQueueTaskPreview = 100

// ListQueueTasks returns recent tasks for a specific queue across all states
// (pending, active, scheduled, retry, archived, completed).
func (h *Handlers) ListQueueTasks(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"pending": []any{}, "active": []any{}, "scheduled": []any{},
			"retry": []any{}, "archived": []any{}, "completed": []any{},
		}})

		return
	}

	queueName := chi.URLParam(r, "queue_name")
	// Tasks are per-queue — "all" (accepted by the bulk ops) is invalid here.
	if queueBulkTargetQueues(queueName) == nil || queueName == "all" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_queue", "queue_name must be critical, default, or low")
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
		IsOrphaned    bool    `json:"is_orphaned,omitempty"`
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
				IsOrphaned: t.IsOrphaned,
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

	// Collect the first page(s) of each state, best-effort (skip
	// non-existent queues). Listing is capped at maxQueueTaskPreview per
	// state; authoritative per-state totals come from GetQueueInfo below.
	pending, _ := listTasksUpTo(h.Inspector.ListPendingTasks, queueName, maxQueueTaskPreview)
	active, _ := listTasksUpTo(h.Inspector.ListActiveTasks, queueName, maxQueueTaskPreview)
	scheduled, _ := listTasksUpTo(h.Inspector.ListScheduledTasks, queueName, maxQueueTaskPreview)
	retry, _ := listTasksUpTo(h.Inspector.ListRetryTasks, queueName, maxQueueTaskPreview)
	archived, _ := listTasksUpTo(h.Inspector.ListArchivedTasks, queueName, maxQueueTaskPreview)
	completed, _ := listTasksUpTo(h.Inspector.ListCompletedTasks, queueName, maxQueueTaskPreview)

	counts := map[string]int{}
	if qi, err := h.Inspector.GetQueueInfo(queueName); err == nil {
		counts = map[string]int{
			"pending":   qi.Pending,
			"active":    qi.Active,
			"scheduled": qi.Scheduled,
			"retry":     qi.Retry,
			"archived":  qi.Archived,
			"completed": qi.Completed,
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"pending":   summarize(pending, "pending"),
			"active":    summarize(active, "active"),
			"scheduled": summarize(scheduled, "scheduled"),
			"retry":     summarize(retry, "retry"),
			"archived":  summarize(archived, "archived"),
			"completed": summarize(completed, "completed"),
			"counts":    counts,
		},
	})
}

func (h *Handlers) RetryDLQTask(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_inspector", "Inspector not available")
		return
	}

	taskID := chi.URLParam(r, "task_id")

	for _, queueName := range []string{"critical", "default", "low"} {
		if err := h.Inspector.RunTask(queueName, taskID); err != nil {
			continue
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": taskID, "status": "retried"}})

		return
	}

	errpkg.WriteError(w, http.StatusNotFound, "task_not_found", "Task not found in any DLQ")
}

// RetryQueueTask retries a specific task within a named queue.
// Uses asynq.Inspector.RunTask to move a retry/archived task back to
// pending. Wired at POST /control/queue/{queue_name}/tasks/{task_id}/retry.
func (h *Handlers) RetryQueueTask(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_inspector", "Inspector not available")
		return
	}

	queueName := chi.URLParam(r, "queue_name")
	if queueBulkTargetQueues(queueName) == nil || queueName == "all" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_queue", "queue_name must be critical, default, or low")
		return
	}

	taskID := chi.URLParam(r, "task_id")
	if taskID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_task", "task_id is required")
		return
	}

	if err := h.Inspector.RunTask(queueName, taskID); err != nil {
		switch {
		case errors.Is(err, asynq.ErrTaskNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "task_not_found", "Task not found in queue")
		case errors.Is(err, asynq.ErrQueueNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "queue_not_found", "Queue not found")
		default:
			// The remaining RunTask failure modes are state conflicts —
			// the task is active or already pending — so a 404 would lie.
			errpkg.WriteError(w, http.StatusConflict, "task_not_actionable", err.Error())
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": taskID, "status": "retried"}})
}

// ArchiveQueueTask moves a single pending, scheduled, or retry task in
// {queue_name} into the dead-letter queue — the recoverable "remove"
// (the task can still be requeued). Wired at
// POST /control/queue/{queue_name}/tasks/{task_id}/archive.
func (h *Handlers) ArchiveQueueTask(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_inspector", "Inspector not available")
		return
	}

	queueName := chi.URLParam(r, "queue_name")
	if queueBulkTargetQueues(queueName) == nil || queueName == "all" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_queue", "queue_name must be critical, default, or low")
		return
	}

	taskID := chi.URLParam(r, "task_id")
	if taskID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_task", "task_id is required")
		return
	}

	if err := h.Inspector.ArchiveTask(queueName, taskID); err != nil {
		switch {
		case errors.Is(err, asynq.ErrTaskNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "task_not_found", "Task not found in queue")
		case errors.Is(err, asynq.ErrQueueNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "queue_not_found", "Queue not found")
		default:
			// Active tasks cannot be archived; completed tasks are already
			// terminal — those failures are state conflicts, not 404s.
			errpkg.WriteError(w, http.StatusConflict, "task_not_actionable", err.Error())
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": taskID, "status": "archived"}})
}

// DeleteQueueTask permanently removes a single task from {queue_name} —
// valid for any non-active state (pending, scheduled, retry, archived,
// completed). Irreversible; the recoverable alternative is
// ArchiveQueueTask. Wired at
// DELETE /control/queue/{queue_name}/tasks/{task_id}.
func (h *Handlers) DeleteQueueTask(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_inspector", "Inspector not available")
		return
	}

	queueName := chi.URLParam(r, "queue_name")
	if queueBulkTargetQueues(queueName) == nil || queueName == "all" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_queue", "queue_name must be critical, default, or low")
		return
	}

	taskID := chi.URLParam(r, "task_id")
	if taskID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_task", "task_id is required")
		return
	}

	if err := h.Inspector.DeleteTask(queueName, taskID); err != nil {
		switch {
		case errors.Is(err, asynq.ErrTaskNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "task_not_found", "Task not found in queue")
		case errors.Is(err, asynq.ErrQueueNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "queue_not_found", "Queue not found")
		default:
			errpkg.WriteError(w, http.StatusConflict, "task_not_actionable", err.Error())
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": taskID, "status": "deleted"}})
}

func (h *Handlers) DiscardDLQTask(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_inspector", "Inspector not available")
		return
	}

	taskID := chi.URLParam(r, "task_id")

	for _, queueName := range []string{"critical", "default", "low"} {
		if err := h.Inspector.DeleteTask(queueName, taskID); err != nil {
			continue
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": taskID, "status": "discarded"}})

		return
	}

	errpkg.WriteError(w, http.StatusNotFound, "task_not_found", "Task not found in any DLQ")
}

// queueBulkTargetQueues resolves {queue_name} into the concrete Asynq
// queue list — "all" expands to every managed queue. Returns nil for an
// unknown queue name so callers can 400.
func queueBulkTargetQueues(name string) []string {
	switch name {
	case "critical", "default", "low":
		return []string{name}
	case "all":
		return []string{"critical", "default", "low"}
	}

	return nil
}

// ArchiveAllQueueTasks bulk-archives every pending, scheduled, and retry
// task in {queue_name} — "clear the queue" without losing the tasks (they
// land in archived/DLQ and can be requeued). Wired at
// POST /control/queue/{queue_name}/archive; queue_name may be "all".
func (h *Handlers) ArchiveAllQueueTasks(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_inspector", "Inspector not available")
		return
	}

	queues := queueBulkTargetQueues(chi.URLParam(r, "queue_name"))
	if queues == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_queue", "queue_name must be critical, default, low, or all")
		return
	}

	// Asynq creates queues lazily on first enqueue and the ArchiveAll*
	// methods return an internal NotFound kind for never-created queues
	// (not the exported ErrQueueNotFound), so gate on Inspector.Queues() —
	// a queue that never existed has nothing to archive and must not
	// surface a spurious per-queue failure. If the lookup itself fails,
	// attempt the ops anyway and surface real errors.
	queueExists, lookupOK := existingQueues(h.Inspector)

	archived := 0

	var failures []string

	for _, queueName := range queues {
		if lookupOK && !queueExists[queueName] {
			continue
		}

		for _, fn := range []func(string) (int, error){
			h.Inspector.ArchiveAllPendingTasks,
			h.Inspector.ArchiveAllScheduledTasks,
			h.Inspector.ArchiveAllRetryTasks,
		} {
			n, err := fn(queueName)
			if err != nil {
				slog.Warn("queue archive-all: partial failure", "queue", queueName, "error", err)
				failures = append(failures, queueName+": "+err.Error())

				continue
			}

			archived += n
		}
	}

	if archived == 0 && len(failures) > 0 {
		errpkg.WriteError(w, http.StatusInternalServerError, "queue_op_failed", "Archive failed: "+strings.Join(failures, "; "))
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"archived": archived,
		"errors":   failures,
	}})
}

// RequeueAllQueueTasks bulk-runs every scheduled, retry, and archived task
// in {queue_name} back to pending so workers process them immediately.
// Wired at POST /control/queue/{queue_name}/requeue; queue_name may be "all".
func (h *Handlers) RequeueAllQueueTasks(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_inspector", "Inspector not available")
		return
	}

	queues := queueBulkTargetQueues(chi.URLParam(r, "queue_name"))
	if queues == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_queue", "queue_name must be critical, default, low, or all")
		return
	}

	// Same never-created-queue quirk as ArchiveAllQueueTasks — a queue
	// that does not exist has nothing to requeue; skip it instead of
	// reporting a spurious per-queue failure.
	queueExists, lookupOK := existingQueues(h.Inspector)

	requeued := 0

	var failures []string

	for _, queueName := range queues {
		if lookupOK && !queueExists[queueName] {
			continue
		}

		for _, fn := range []func(string) (int, error){
			h.Inspector.RunAllScheduledTasks,
			h.Inspector.RunAllRetryTasks,
			h.Inspector.RunAllArchivedTasks,
		} {
			n, err := fn(queueName)
			if err != nil {
				slog.Warn("queue requeue-all: partial failure", "queue", queueName, "error", err)
				failures = append(failures, queueName+": "+err.Error())

				continue
			}

			requeued += n
		}
	}

	if requeued == 0 && len(failures) > 0 {
		errpkg.WriteError(w, http.StatusInternalServerError, "queue_op_failed", "Requeue failed: "+strings.Join(failures, "; "))
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"requeued": requeued, "errors": failures}})
}

// PurgeQueueTasks permanently deletes every archived (dead-letter) task in
// {queue_name} — irreversible; the recoverable alternative is
// ArchiveAllQueueTasks followed by RequeueAllQueueTasks. Wired at
// POST /control/queue/{queue_name}/purge; queue_name may be "all".
func (h *Handlers) PurgeQueueTasks(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_inspector", "Inspector not available")
		return
	}

	if !requireBulkConfirm(w, r, "purge") {
		return
	}

	queues := queueBulkTargetQueues(chi.URLParam(r, "queue_name"))
	if queues == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_queue", "queue_name must be critical, default, low, or all")
		return
	}

	// Asynq creates queues lazily on first enqueue and DeleteAllArchivedTasks
	// returns an internal NotFound kind for never-created queues (not the
	// exported ErrQueueNotFound), so gate on Inspector.Queues() — a queue
	// that never existed is "0 deleted", not a failure. If the lookup itself
	// fails, attempt deletes anyway and surface real errors.
	queueExists, lookupOK := existingQueues(h.Inspector)

	purged := 0

	var failures []string

	for _, queueName := range queues {
		if lookupOK && !queueExists[queueName] {
			continue
		}

		n, err := h.Inspector.DeleteAllArchivedTasks(queueName)
		if err != nil {
			slog.Warn("queue purge: partial failure", "queue", queueName, "error", err)
			failures = append(failures, queueName+": "+err.Error())

			continue
		}

		purged += n
	}

	if purged == 0 && len(failures) > 0 {
		errpkg.WriteError(w, http.StatusInternalServerError, "queue_op_failed", "Purge failed: "+strings.Join(failures, "; "))
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"purged": purged, "errors": failures}})
}

// requireBulkConfirm enforces the {"confirm":true} body contract on
// irreversible bulk queue ops (purge, DLQ discard-all) — a bare POST
// (script, replay, or fat-fingered fetch) cannot drain the dead-letter
// queues. An absent body decodes as unconfirmed. Writes the error and
// returns false when the gate fails; op names the operation in the
// refusal message.
func requireBulkConfirm(w http.ResponseWriter, r *http.Request, op string) bool {
	var body struct {
		Confirm bool `json:"confirm"`
	}

	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return false
		}
	}

	if !body.Confirm {
		errpkg.WriteError(w, http.StatusBadRequest, "confirmation_required", "Queue "+op+" permanently deletes queued tasks; resend with {\"confirm\": true}")
		return false
	}

	return true
}

// DiscardAllDLQ discards every task across the dead-letter queues —
// permanently; gated by requireBulkConfirm like PurgeQueueTasks. Uses
// DeleteAllArchivedTasks (atomic, unbounded) — ListArchivedTasks is
// capped at 30 rows per call and would silently strand the remainder.
func (h *Handlers) DiscardAllDLQ(w http.ResponseWriter, r *http.Request) {
	if h.Inspector == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_inspector", "Inspector not available")
		return
	}

	if !requireBulkConfirm(w, r, "discard-all") {
		return
	}

	queueExists, lookupOK := existingQueues(h.Inspector)

	totalDiscarded := 0

	var failures []string

	for _, queueName := range []string{"critical", "default", "low"} {
		if lookupOK && !queueExists[queueName] {
			continue
		}

		n, err := h.Inspector.DeleteAllArchivedTasks(queueName)
		if err != nil {
			slog.Warn("dlq discard-all: delete failed", "queue", queueName, "error", err)
			failures = append(failures, queueName+": "+err.Error())

			continue
		}

		totalDiscarded += n
	}

	if totalDiscarded == 0 && len(failures) > 0 {
		errpkg.WriteError(w, http.StatusInternalServerError, "queue_op_failed", "Discard failed: "+strings.Join(failures, "; "))
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"discarded": totalDiscarded, "errors": failures}})
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

// CacheStats is an exported method.
func (h *Handlers) CacheStats(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch cache stats")

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
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": stats})
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

// FlushCache is an exported method.
func (h *Handlers) FlushCache(w http.ResponseWriter, r *http.Request) {
	if h.RBAC != nil {
		h.RBAC.InvalidateAllCache(r.Context())
	}

	_, ok := requireUser(w, r)
	if ok && h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context())
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"message": "Permission and settings cache flushed"},
	})
}

// ---------------------------------------------------------------------------
// Security Handlers
// ---------------------------------------------------------------------------

// SecurityOverview is an exported method.
func (h *Handlers) SecurityOverview(w http.ResponseWriter, r *http.Request) {
	blocks, err := h.SpamStore.GetActiveIPBlocks(r.Context())
	if err != nil {
		slog.Error("control: security overview", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch security overview")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"active_ip_blocks": len(blocks),
		},
	})
}

// ListIPBlocks is an exported method.
func (h *Handlers) ListIPBlocks(w http.ResponseWriter, r *http.Request) {
	blocks, err := h.SpamStore.ListIPBlocks(r.Context(), 200, pgtype.Timestamptz{})
	if err != nil {
		slog.Error("control: list ip blocks", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list IP blocks")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": blocks})
}

// ListSecurityScanResults is an exported method.
func (h *Handlers) ListSecurityScanResults(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 0 {
		page = 0
	}

	limit := 20
	offset := int32(page * limit) //nolint:gosec // G115: bounded

	severity := r.URL.Query().Get("severity")

	var results []map[string]any

	var total int64

	countErr := h.DB.Reader.QueryRow(r.Context(), `SELECT count(*) FROM security_scan_results WHERE ($1::text = '' OR severity = $1)`, severity).Scan(&total)
	if countErr != nil {
		slog.Warn("control: count security scan results", "error", countErr)
	}

	rows, err := h.DB.Reader.Query(r.Context(), `SELECT id, scan_type, severity, finding, meta, resolved_at, created_at
		 FROM security_scan_results
		 WHERE ($1::text = '' OR severity = $1)
		 ORDER BY created_at DESC, id DESC
		 LIMIT $2 OFFSET $3`, severity, int32(limit), offset)
	if err != nil {
		slog.Error("control: list security scan results", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list security scan results")

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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"results":     results,
			"page":        page,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// TriggerSecurityScan is an exported method.
func (h *Handlers) TriggerSecurityScan(w http.ResponseWriter, r *http.Request) {
	if h.Client == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "queue_unavailable", "Task queue not available")
		return
	}

	payload, err := json.Marshal(map[string]any{})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create scan task")
		return
	}

	// AUDIT-BE-TASK-008: use the constant worker.TypeSecurityScan instead of
	// the hardcoded string "security:scan" (DRY — prevents silent task drops
	// if the constant value changes).
	task := asynq.NewTask(worker.TypeSecurityScan, payload)

	info, err := h.Client.Enqueue(task)
	if err != nil {
		slog.Error("control: trigger security scan", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to enqueue security scan")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"task_id": info.ID,
			"status":  "enqueued",
		},
	})
}

// CreateIPBlock is an exported method.
func (h *Handlers) CreateIPBlock(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		CIDR      string  `json:"cidr"`
		BlockType string  `json:"block_type"`
		Reason    *string `json:"reason"`
		ExpiresAt *string `json:"expires_at"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.CIDR == "" || input.BlockType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "cidr and block_type are required")
		return
	}
	// BUG-006: validate block_type against the §41 enum so an invalid value
	// (e.g. "global") is rejected at the API boundary, matching the admin
	// spam handler's validation.
	if !spam.IsValidBlockType(input.BlockType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_block_type",
			"block_type must be one of: full, registration, posting, api")
		return
	}

	createInput := spam.CreateIPBlockInput{
		CIDR:      input.CIDR,
		BlockType: input.BlockType,
		Reason:    input.Reason,
		BlockedBy: userID,
	}

	if input.ExpiresAt != nil && *input.ExpiresAt != "" {
		ts, err := timeparse.ParseTimestamptz(*input.ExpiresAt)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_expires_at",
				"expires_at must be an RFC3339 timestamp")
			return
		}

		createInput.ExpiresAt = ts
	}

	block, err := h.SpamStore.CreateIPBlock(r.Context(), createInput)
	if err != nil {
		slog.Error("control: create ip block", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create IP block")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": block})
}

// DeleteIPBlock is an exported method.
func (h *Handlers) DeleteIPBlock(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid IP block ID")
		return
	}

	if err := h.SpamStore.DeleteIPBlock(r.Context(), id); err != nil {
		slog.Error("control: delete ip block", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete IP block")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
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
	threshold := 0.0

	if h.Settings != nil {
		if v, err := h.Settings.GetSetting(r.Context(), aiBudgetThresholdKey); err == nil {
			threshold = parseBudgetUSD(v)
		}
	}

	// Aggregate month-to-date spend from ai_usage_log. When the AI store is
	// not wired (e.g. tests), fall back to 0 spend with empty breakdowns.
	monthToDateCost := 0.0
	byProvider := []any{}
	byTaskType := []any{}

	if h.AIStore != nil {
		summary, err := h.AIStore.GetAIUsageSummary(r.Context(), 30, "")
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
	var input struct {
		BudgetThresholdUSD *float64 `json:"budget_threshold_usd"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.BudgetThresholdUSD == nil || *input.BudgetThresholdUSD < 0 || *input.BudgetThresholdUSD > 1000000 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_threshold", "budget_threshold_usd must be between 0 and 1,000,000")
		return
	}

	threshold := *input.BudgetThresholdUSD

	data, _ := json.Marshal(threshold)
	if _, err := h.DB.Writer.Exec(r.Context(), `INSERT INTO system_settings (key, value, updated_at) VALUES ($1, $2, NOW()) ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = NOW()`, aiBudgetThresholdKey, string(data)); err != nil {
		slog.Error("control: update ai budget", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update AI budget threshold")

		return
	}

	if h.Settings != nil {
		h.Settings.InvalidateSettings(r.Context())
	}

	// Re-aggregate spend so the response reflects the current state.
	monthToDateCost := 0.0
	byProvider := []any{}
	byTaskType := []any{}

	if h.AIStore != nil {
		summary, err := h.AIStore.GetAIUsageSummary(r.Context(), 30, "")
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
		ID         pgtype.UUID    `json:"id"`
		Timestamp  time.Time      `json:"timestamp"`
		Actor      *auditLogActor `json:"actor"`
		ActionType string         `json:"action_type"`
		EntityType *string        `json:"entity_type"`
		EntityID   *string        `json:"entity_id"`

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

	// Count the total matching rows (same WHERE, no LIMIT/OFFSET) so the
	// control panel can render correct pagination. The count uses the filter
	// args built above; the pagination args are appended only for the SELECT.
	countQuery := "SELECT count(*) FROM audit_logs al\n\t\tLEFT JOIN users u ON u.id = al.actor_user_id\n\t\tWHERE 1=1" + where

	var total int64

	if err := h.DB.Reader.QueryRow(r.Context(), countQuery, args...).Scan(&total); err != nil {
		slog.Error("control: count audit logs", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count audit logs")

		return
	}

	query := fmt.Sprintf(`SELECT al.id, al.created_at, al.actor_user_id, u.username,
		al.action_type, al.target_type, al.target_id::text, al.ip_address::text, al.impersonated_user_id::text, al.metadata
		FROM audit_logs al
		LEFT JOIN users u ON u.id = al.actor_user_id
		WHERE 1=1%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d`, where, sortBy, sortDir, argIdx, argIdx+1)

	args = append(args, perPage, offset)

	rows, err := h.DB.Reader.Query(r.Context(), query, args...)
	if err != nil {
		slog.Error("control: list audit logs", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list audit logs")

		return
	}
	defer rows.Close()

	var logs []auditLog

	for rows.Next() {
		var l auditLog

		var actorID, username sql.NullString
		if err := rows.Scan(&l.ID, &l.Timestamp, &actorID, &username,
			&l.ActionType, &l.EntityType, &l.EntityID, &l.IPAddress, &l.ImpersonatedUserID, &l.Metadata); err != nil {
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

	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	if totalPages < 1 {
		totalPages = 1
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"data":        logs,
			"total":       total,
			"page":        page,
			"per_page":    perPage,
			"total_pages": totalPages,
		},
	})
}

// GetAuditLogFilters returns the distinct action_type and target_type values
// observed in audit_logs. The control panel uses these to populate the action
// and target filter dropdowns so the lists stay in sync with the backend.
func (h *Handlers) GetAuditLogFilters(w http.ResponseWriter, r *http.Request) {
	actionTypes := []string{}

	rows, err := h.DB.Reader.Query(r.Context(), `SELECT DISTINCT action_type FROM audit_logs ORDER BY action_type`)
	if err != nil {
		slog.Error("control: list audit log action types", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list audit log filters")

		return
	}

	defer rows.Close()

	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			slog.Error("control: scan audit log action type", "error", err)
			continue
		}

		actionTypes = append(actionTypes, action)
	}

	targetTypes := []string{}

	rows2, err := h.DB.Reader.Query(r.Context(), `SELECT DISTINCT target_type FROM audit_logs WHERE target_type IS NOT NULL ORDER BY target_type`)
	if err != nil {
		slog.Error("control: list audit log target types", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list audit log filters")

		return
	}

	defer rows2.Close()

	for rows2.Next() {
		var target sql.NullString
		if err := rows2.Scan(&target); err != nil {
			slog.Error("control: scan audit log target type", "error", err)
			continue
		}

		if target.Valid {
			targetTypes = append(targetTypes, target.String)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"action_types": actionTypes,
			"target_types": targetTypes,
		},
	})
}

// DeleteAuditLog handles DELETE /api/v1/control/audit-logs/{id}.
// Hard-deletes a single audit log entry by id. The control panel audit log is
// a full cross-platform super-admin view (see ListAuditLogs), so the lookup is
// not platform-scoped. Returns 200 even if the row did not exist.
func (h *Handlers) DeleteAuditLog(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid audit log ID")
		return
	}

	if _, err := h.DB.Writer.Exec(r.Context(), `DELETE FROM audit_logs WHERE id = $1`, id); err != nil {
		slog.Error("control: delete audit log", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete audit log")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
// Bulk-deletes audit log entries. By default (no body) it purges logs older
// than the retention window. Scoped purges (`before` / `action_type`) are
// allowed without confirmation; an unscoped full clear requires `confirm: true`.
func (h *Handlers) ClearAuditLogs(w http.ResponseWriter, r *http.Request) {
	var input AuditLogClearInput
	if body, _ := io.ReadAll(r.Body); len(body) > 0 {
		if err := json.Unmarshal(body, &input); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
			return
		}
	}

	var where string

	var args []any

	switch {
	case input.Before != nil && *input.Before != "":
		ts, err := time.Parse(time.RFC3339, *input.Before)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_before", "before must be an RFC3339 timestamp")
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
			retentionDays := h.Settings.GetIntOr(r.Context(), "audit.log_retention_days", 365)
			cutoff := time.Now().AddDate(0, 0, -retentionDays)
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
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to clear audit logs")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"cleared": tag.RowsAffected()},
	})
}

// ---------------------------------------------------------------------------
// Deployment Handlers (stubs)
// ---------------------------------------------------------------------------

// ListDeployments returns the current deployment version info for the control panel.
func (h *Handlers) ListDeployments(w http.ResponseWriter, r *http.Request) {
	version := h.DeploymentVersion
	if version == "" {
		version = BuildVersion
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": []map[string]any{
			{
				"version":     version,
				"status":      "active",
				"deployed_at": h.StartTime.Format(time.RFC3339),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// API Key Management Handlers (delegate to publicapi)
// ---------------------------------------------------------------------------

// CreateAPIKey is an exported method.
func (h *Handlers) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.CreateKey(w, r)
}

// GetAPIKey is an exported method.
func (h *Handlers) GetAPIKey(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.GetAPIKey(w, r)
}

// UpdateAPIKey is an exported method.
func (h *Handlers) UpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.UpdateAPIKeyStatus(w, r)
}

// DeleteAPIKey is an exported method.
func (h *Handlers) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.DeleteKeyPermanently(w, r)
}

// DeleteAPIKeysBatch is an exported method.
func (h *Handlers) DeleteAPIKeysBatch(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.DeleteKeysPermanently(w, r)
}

// GetAPIKeyAnalytics is an exported method.
func (h *Handlers) GetAPIKeyAnalytics(w http.ResponseWriter, r *http.Request) {
	h.PublicAPI.GetAPIKeyAnalytics(w, r)
}

// ---------------------------------------------------------------------------
// Cron / Scheduler CRUD (custom entries stored in Valkey, synced by worker)
// ---------------------------------------------------------------------------

// cronCustomKey returns the platform-scoped Valkey key for custom cron jobs.
// Platform-scoping (Rule 4 / §4.2 / §9) prevents one platform's control panel from
// seeing or mutating another platform's custom scheduler entries.
func cronCustomKey(ctx context.Context) string {
	return valkey.MustKey("cron:custom:%s", pgtype.UUID{}.String())
}

// cronPausedKey returns the platform-scoped Valkey key for paused scheduler entries.
func cronPausedKey(ctx context.Context) string {
	return valkey.MustKey("cron:paused:%s", pgtype.UUID{}.String())
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": jobs})
}

// CreateCronJob handles POST /api/v1/control/cron/jobs.
// Creates a custom cron entry stored in Valkey. The worker process picks it up.
func (h *Handlers) CreateCronJob(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_valkey", "Valkey not available")
		return
	}

	var input struct {
		Type    string `json:"type"`
		Spec    string `json:"spec"`
		Payload string `json:"payload"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Type == "" || input.Spec == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "type and spec are required")
		return
	}
	// L5: Validate payload JSON if provided (defense in depth — the
	// frontend also validates, but the backend must not accept invalid JSON).
	if input.Payload != "" {
		var js any
		if json.Unmarshal([]byte(input.Payload), &js) != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_payload", "Payload must be valid JSON")
			return
		}
	}

	// Generate a unique ID for the custom entry
	id := fmt.Sprintf("custom_%d", time.Now().UnixNano())
	entry := cronCustomEntry{Type: input.Type, Spec: input.Spec, Payload: input.Payload}

	data, err := json.Marshal(entry)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "marshal_error", "Failed to encode entry")
		return
	}

	if err := h.Valkey.HSet(r.Context(), cronCustomKey(r.Context()), id, string(data)).Err(); err != nil {
		slog.Error("control: create cron job", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "redis_error", "Failed to save cron job")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{"id": id, "type": entry.Type, "spec": entry.Spec},
	})
}

// UpdateCronJob handles PATCH /api/v1/control/cron/jobs/{id}.
// Updates a custom cron entry in Valkey.
func (h *Handlers) UpdateCronJob(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_valkey", "Valkey not available")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_id", "Job ID is required")
		return
	}

	// Only custom entries can be edited
	raw, err := h.Valkey.HGet(r.Context(), cronCustomKey(r.Context()), id).Result()
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Cron job not found (only custom jobs can be edited)")
		return
	}

	var existing cronCustomEntry
	if json.Unmarshal([]byte(raw), &existing) != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "decode_error", "Failed to read existing entry")
		return
	}

	var input struct {
		Type    *string `json:"type"`
		Spec    *string `json:"spec"`
		Payload *string `json:"payload"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
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
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_payload", "Payload must be valid JSON")
				return
			}
		}

		existing.Payload = *input.Payload
	}

	if existing.Type == "" || existing.Spec == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "type and spec cannot be empty")
		return
	}

	data, err := json.Marshal(existing)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "marshal_error", "Failed to encode entry")
		return
	}

	if err := h.Valkey.HSet(r.Context(), cronCustomKey(r.Context()), id, string(data)).Err(); err != nil {
		slog.Error("control: update cron job", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "redis_error", "Failed to update cron job")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"id": id, "type": existing.Type, "spec": existing.Spec},
	})
}

// DeleteCronJob handles DELETE /api/v1/control/cron/jobs/{id}.
// Deletes a custom cron entry from Valkey or pauses a system entry (soft-delete).
func (h *Handlers) DeleteCronJob(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_valkey", "Valkey not available")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_id", "Job ID is required")
		return
	}

	// Try custom entry first
	deleted, err := h.Valkey.HDel(r.Context(), cronCustomKey(r.Context()), id).Result()
	if err != nil {
		slog.Error("control: delete cron job", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "redis_error", "Failed to delete cron job")

		return
	}

	if deleted > 0 {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "status": "deleted"}})
		return
	}

	// For system entries, we can't truly delete (managed by code).
	// Instead, mark as paused so it won't run.
	if err := h.Valkey.SAdd(r.Context(), cronPausedKey(r.Context()), id).Err(); err != nil {
		slog.Error("control: pause system cron job", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "redis_error", "Failed to pause system cron job")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"id": id, "status": "paused", "note": "System jobs cannot be deleted; entry has been paused instead."},
	})
}

// PauseCronJob handles POST /api/v1/control/cron/jobs/{id}/pause.
func (h *Handlers) PauseCronJob(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_valkey", "Valkey not available")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_id", "Job ID is required")
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
				errpkg.WriteError(w, http.StatusInternalServerError, "redis_error", "Failed to pause cron job")

				return
			}

			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "status": "paused"}})

			return
		}
	}

	// System entry — add to paused set
	if err := h.Valkey.SAdd(r.Context(), cronPausedKey(r.Context()), id).Err(); err != nil {
		slog.Error("control: pause system cron job", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "redis_error", "Failed to pause cron job")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "status": "paused"}})
}

// UnpauseCronJob handles POST /api/v1/control/cron/jobs/{id}/unpause.
func (h *Handlers) UnpauseCronJob(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_valkey", "Valkey not available")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_id", "Job ID is required")
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
				errpkg.WriteError(w, http.StatusInternalServerError, "redis_error", "Failed to unpause cron job")

				return
			}

			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "status": "active"}})

			return
		}
	}

	// System entry — remove from paused set
	if err := h.Valkey.SRem(r.Context(), cronPausedKey(r.Context()), id).Err(); err != nil {
		slog.Error("control: unpause system cron job", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "redis_error", "Failed to unpause cron job")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "status": "active"}})
}

// TriggerCronJob handles POST /api/v1/control/cron/jobs/{id}/trigger.
// Enqueues the scheduler entry's task immediately (one-off run).
func (h *Handlers) TriggerCronJob(w http.ResponseWriter, r *http.Request) {
	if h.Client == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_client", "Asynq client not available")
		return
	}

	if h.Inspector == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_inspector", "Asynq inspector not available")
		return
	}

	id := chi.URLParam(r, "id")

	entries, err := h.Inspector.SchedulerEntries()
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "scheduler_error", "Failed to load cron jobs")
		return
	}

	for _, e := range entries {
		if e.ID != id || e.Task == nil {
			continue
		}

		if _, err := h.Client.Enqueue(e.Task, asynq.Queue(worker.QueueForTask(e.Task.Type()))); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_error", "Failed to trigger cron job")
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": e.ID, "status": "enqueued"}})

		return
	}

	// Check custom entries — trigger by re-enqueuing via Asynq client
	if h.Valkey != nil {
		if raw, err := h.Valkey.HGet(r.Context(), cronCustomKey(r.Context()), id).Result(); err == nil {
			var ce cronCustomEntry
			if json.Unmarshal([]byte(raw), &ce) == nil && ce.Type != "" {
				task := asynq.NewTask(ce.Type, []byte(ce.Payload))
				if _, err := h.Client.Enqueue(task, asynq.Queue(worker.QueueForTask(ce.Type))); err != nil {
					errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_error", "Failed to trigger cron job")
					return
				}

				errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "status": "enqueued"}})

				return
			}
		}
	}

	errpkg.WriteError(w, http.StatusNotFound, "not_found", "Cron job not found")
}

// ListTaskTypes handles GET /control/task-types — returns the full Asynq task
// type catalog with name, tier, and description for the cron job editor UI.
func (h *Handlers) ListTaskTypes(w http.ResponseWriter, r *http.Request) {
	taskTypes := worker.TaskTypeCatalog()
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": taskTypes})
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
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"presets": presets,
			"tiers":   tiers,
		},
	})
}

func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

func requireUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
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

// ListPermissionNodes is an exported method.
func (h *Handlers) ListPermissionNodes(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Reader.Query(r.Context(), `SELECT key, description FROM permissions ORDER BY key`)
	if err != nil {
		slog.Error("control: list permission nodes", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list permission nodes")

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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": nodes})
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

// oauthClientsKey is the shared Valkey key holding the JSON array of
// Control-managed OAuth client entries. Must match the catalog entry
// jc:oauth_clients — the auth engine (internal/auth/oauth.go) reads this
// same key to resolve providers without a redeploy.
func oauthClientsKey() string {
	return valkey.MustKey("oauth_clients")
}

const oauthClientSecretMasked = "••••••••" // #nosec

func maskOAuthClient(c oauthClientEntry) oauthClientEntry {
	c.ClientSecret = oauthClientSecretMasked
	return c
}

// ListOAuthClients is an exported method.
func (h *Handlers) ListOAuthClients(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	raw, err := h.Valkey.Get(r.Context(), oauthClientsKey()).Result()
	if err != nil || raw == "" {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	var clients []oauthClientEntry
	if json.Unmarshal([]byte(raw), &clients) != nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}
	// Secrets are never returned in list views; they are revealed once on
	// create/rotate and are otherwise masked in the UI.
	masked := make([]oauthClientEntry, len(clients))
	for i, c := range clients {
		masked[i] = maskOAuthClient(c)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": masked})
}

// ListOAuthProviders handles GET /control/oauth-providers — returns the list of
// OAuth providers supported by the engine (google, github, apple, facebook).
func (h *Handlers) ListOAuthProviders(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": supportedOAuthProviders})
}

// GetOAuthClient is an exported method.
func (h *Handlers) GetOAuthClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_id", "id is required")
		return
	}

	clients := h.loadOAuthClients(r)
	for _, c := range clients {
		if c.ID == id {
			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": maskOAuthClient(c)})
			return
		}
	}

	errpkg.WriteError(w, http.StatusNotFound, "not_found", "OAuth client not found")
}

// CreateOAuthClient is an exported method.
func (h *Handlers) CreateOAuthClient(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name         string `json:"name"`
		Provider     string `json:"provider"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RedirectURI  string `json:"redirect_uri"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" || input.Provider == "" || input.ClientID == "" || input.ClientSecret == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "name, provider, client_id, and client_secret are required")
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_provider", "Unsupported OAuth provider: "+input.Provider)
		return
	}

	clients := h.loadOAuthClients(r)
	entry := oauthClientEntry{
		ID:           "oauth_" + randomHex(16),
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
		errpkg.WriteError(w, http.StatusInternalServerError, "save_error", "Failed to save OAuth client")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": entry})
}

// UpdateOAuthClient is an exported method.
func (h *Handlers) UpdateOAuthClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_id", "id is required")
		return
	}

	var input struct {
		Name         *string `json:"name"`
		ClientSecret *string `json:"client_secret"`
		RedirectURI  *string `json:"redirect_uri"`
		Enabled      *bool   `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
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
				errpkg.WriteError(w, http.StatusInternalServerError, "save_error", "Failed to update OAuth client")
				return
			}

			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": maskOAuthClient(clients[i])})

			return
		}
	}

	errpkg.WriteError(w, http.StatusNotFound, "not_found", "OAuth client not found")
}

func (h *Handlers) DeleteOAuthClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_id", "id is required")
		return
	}

	clients := h.loadOAuthClients(r)
	for i, c := range clients {
		if c.ID == id {
			clients = append(clients[:i], clients[i+1:]...)
			if err := h.saveOAuthClients(r, clients); err != nil {
				errpkg.WriteError(w, http.StatusInternalServerError, "save_error", "Failed to delete OAuth client")
				return
			}

			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})

			return
		}
	}

	errpkg.WriteError(w, http.StatusNotFound, "not_found", "OAuth client not found")
}

// RotateOAuthClientSecret is an exported method.
func (h *Handlers) RotateOAuthClientSecret(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_id", "id is required")
		return
	}

	clients := h.loadOAuthClients(r)
	for i := range clients {
		if clients[i].ID == id {
			// Generate a new secret from crypto/rand — a timestamp-derived
			// secret is predictable and would let an attacker precompute it.
			// The new secret is returned exactly once; list/get always mask it.
			newSecret := "sec_" + randomHex(32)

			clients[i].ClientSecret = newSecret
			if err := h.saveOAuthClients(r, clients); err != nil {
				errpkg.WriteError(w, http.StatusInternalServerError, "save_error", "Failed to rotate secret")
				return
			}

			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": id, "status": "rotated", "client_secret": newSecret}})

			return
		}
	}

	errpkg.WriteError(w, http.StatusNotFound, "not_found", "OAuth client not found")
}

// randomHex returns n bytes of crypto/rand entropy as a lowercase hex string.
// On the (practically impossible) rand failure it returns an empty string —
// callers use it for identifiers/secrets where an empty value is rejected by
// downstream validation rather than silently predictable.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return ""
	}

	return hex.EncodeToString(b)
}

func (h *Handlers) loadOAuthClients(r *http.Request) []oauthClientEntry {
	if h.Valkey == nil {
		return []oauthClientEntry{}
	}

	raw, err := h.Valkey.Get(r.Context(), oauthClientsKey()).Result()
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
		return errors.New("valkey not available")
	}

	// #nosec G117 — OAuth client secrets are persisted in Valkey; this is the
	// intended storage path. List/get handlers mask the secret before returning
	// it to the UI.
	data, err := json.Marshal(clients)
	if err != nil {
		return err
	}

	return h.Valkey.Set(r.Context(), oauthClientsKey(), string(data), 0).Err()
}

// ---------------------------------------------------------------------------
// Extensions Handlers — the /control/extensions catalog is the single manager
// for ALL feature flags: the union of features.Registry keys and every
// feature_flags row (admin-created keys outside the Registry list under
// category "Custom"). All enabled/disabled state lives in feature_flags —
// the old system_settings "extensions.*" fallback is gone.
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §6.3.
// ---------------------------------------------------------------------------

// extensionEntry is the per-item shape returned by GET /control/extensions.
// The control panel is privileged — Surfaces carries the full per-surface
// rules (state/audience/geo) that the public details endpoint omits.
type extensionEntry struct {
	Key              string `json:"key"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Enabled          bool   `json:"enabled"`
	Version          string `json:"version"`
	SettingKey       string `json:"setting_key"`
	SettingsCategory string `json:"settings_category"`
	// Category groups entries in the UI (e.g. "Content", "Social", "AI").
	// Flags created outside the Registry land in "Custom".
	Category string `json:"category"`
	// RolloutPercentage is the gradual rollout percentage (0-100) from the
	// feature_flags config blob. 100 = fully on, 0 = off, 1-99 = deterministic
	// per-platform hash bucketing (BL-013 / Section 42).
	RolloutPercentage int `json:"rollout_percentage"`
	// Phase is the rollout phase — the stored config value when set, else
	// derived ("shipped" for enabled flags, "planned" for disabled ones).
	Phase         string                          `json:"phase"`
	ShowOnRoadmap bool                            `json:"show_on_roadmap"`
	Surfaces      map[string]features.SurfaceRule `json:"surfaces"`
	// LinkedRoadmapSlug is the slug of the roadmap_items row linked via
	// feature_flag_key. Omitted when no item is linked.
	LinkedRoadmapSlug string `json:"linked_roadmap_slug,omitempty"`
	// Internal excludes the flag from the public flag projections (§42).
	Internal bool `json:"internal"`
	// StoredSurfaces is the raw stored surfaces map (unmerged with registry
	// DefaultSurfaces) — the manage UI needs it to tell an explicit stored
	// {state:"visible"} override apart from a default-visible surface.
	StoredSurfaces map[string]features.SurfaceRule `json:"stored_surfaces,omitempty"`
}

// Derived phases shown for flags whose config carries no explicit phase —
// the same derivation eval applies to absent phases (spec §4.1).
const (
	extPhaseShipped = "shipped"
	extPhasePlanned = "planned"
)

// ListExtensions handles GET /control/extensions. Every flag the control
// panel manages appears exactly once — Registry keys in curated order first,
// then admin-created rows not in the Registry (sorted by key). The page
// polls every 30s over ~60 flags, so reads are batched: one AllFlagRows
// aggregate call (jc:flags:_all cache — no per-request Postgres hit) plus
// one ListRoadmapItemsByFlagKeys query, merged in Go. Per-flag
// IsEnabled/GetFlagConfig/roadmap lookups are deliberately avoided (N+1).
func (h *Handlers) ListExtensions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var rows []features.FlagRow
	if h.FeatureChecker != nil {
		rows = h.FeatureChecker.AllFlagRows(ctx)
	}

	rowsByKey := make(map[string]features.FlagRow, len(rows))
	for _, row := range rows {
		rowsByKey[row.Key] = row
	}

	// Union: every Registry key, then stored rows for keys not registered.
	keys := make([]string, 0, len(features.Registry)+len(rows))
	seen := make(map[string]bool, len(features.Registry)+len(rows))

	for _, f := range features.Registry {
		keys = append(keys, f.Key)
		seen[f.Key] = true
	}

	customKeys := make([]string, 0, 8)

	for _, row := range rows {
		if !seen[row.Key] {
			customKeys = append(customKeys, row.Key)
			seen[row.Key] = true
		}
	}

	sort.Strings(customKeys)

	keys = append(keys, customKeys...)

	// One batched roadmap lookup — unlinked flags simply have no map entry.
	slugByFlag := h.linkedRoadmapSlugs(ctx, keys)

	result := make([]extensionEntry, 0, len(keys))

	for _, key := range keys {
		result = append(result, buildExtensionEntry(key, rowsByKey[key], slugByFlag[key]))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// buildExtensionEntry merges a flag key's Registry defaults with its stored
// feature_flags row (when present) into the extensions-list item. The stored
// row's enabled state is the raw column value — not the rollout-bucketed
// eval result — so the admin sees the flag's actual on/off alongside its
// rollout percentage. Keys outside the Registry list as category "Custom".
func buildExtensionEntry(key string, row features.FlagRow, roadmapSlug string) extensionEntry {
	entry := extensionEntry{
		Key:               key,
		SettingKey:        key, // the feature_flags key is the storage key now
		Version:           "",
		RolloutPercentage: 100,
		Surfaces:          map[string]features.SurfaceRule{},
	}

	if f, ok := features.RegistryLookup(key); ok {
		entry.Name = f.Label
		entry.Description = f.Description
		entry.Category = f.Category
		entry.SettingsCategory = f.SettingsCategory
		entry.Enabled = f.Default

		for surface, state := range f.DefaultSurfaces {
			entry.Surfaces[surface] = features.SurfaceRule{State: state}
		}
	} else {
		entry.Name = humanizeExtensionKey(key)
		entry.Category = "Custom"
	}

	var cfg features.FlagConfig

	if row.Key != "" {
		entry.Enabled = row.Enabled
		cfg = decodeFlagConfig(row.Config)
	}

	if cfg.Name != "" {
		entry.Name = cfg.Name
	}

	if cfg.Description != "" {
		entry.Description = cfg.Description
	}

	if cfg.RolloutPercentage != 0 {
		entry.RolloutPercentage = cfg.RolloutPercentage
	}

	entry.ShowOnRoadmap = cfg.ShowOnRoadmap
	entry.Internal = cfg.Internal
	entry.StoredSurfaces = cfg.Surfaces
	entry.Phase = extensionPhase(entry.Enabled, cfg)

	for surface, rule := range cfg.Surfaces {
		entry.Surfaces[surface] = rule
	}

	if roadmapSlug != "" {
		entry.LinkedRoadmapSlug = roadmapSlug
	}

	return entry
}

// linkedRoadmapSlugs batch-resolves roadmap_items.feature_flag_key → slug
// for the whole catalog in one query — the linked_roadmap_slug join for the
// extensions list. Returns an empty map when the read pool is unavailable or
// the query fails: the list must still render without roadmap links.
func (h *Handlers) linkedRoadmapSlugs(ctx context.Context, keys []string) map[string]string {
	slugs := make(map[string]string)
	if h.DB == nil || h.DB.Reader == nil || len(keys) == 0 {
		return slugs
	}

	items, err := gen.New(h.DB.Reader).ListRoadmapItemsByFlagKeys(ctx, keys)
	if err != nil {
		slog.Warn("control: roadmap link lookup failed", "error", err)
		return slugs
	}

	for _, item := range items {
		if item.FeatureFlagKey != nil {
			slugs[*item.FeatureFlagKey] = item.Slug
		}
	}

	return slugs
}

// decodeFlagConfig parses a feature_flags.config jsonb blob. Malformed or
// absent blobs decode to a zero config — the same tolerance GetFlagConfig
// applies.
func decodeFlagConfig(raw []byte) features.FlagConfig {
	if len(raw) == 0 {
		return features.FlagConfig{}
	}

	var cfg features.FlagConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return features.FlagConfig{}
	}

	return cfg
}

// extensionPhase derives the displayed phase: the config's explicit phase
// when set, else "shipped" for enabled flags and "planned" for disabled
// ones — the same derivation eval applies to absent phases.
func extensionPhase(enabled bool, cfg features.FlagConfig) string {
	if cfg.Phase != "" {
		return features.DisplayPhase(cfg.Phase, enabled)
	}

	if enabled {
		return extPhaseShipped
	}

	return extPhasePlanned
}

// humanizeExtensionKey turns a dotted/snake_case flag key into a readable
// label for admin-created flags that carry no config name (mirrors the
// humanizeFlagName helper in the features package).
func humanizeExtensionKey(key string) string {
	key = strings.ReplaceAll(key, ".", "_")

	parts := strings.Split(key, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}

		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}

	return strings.Join(parts, " ")
}

// extensionRow reads the stored feature_flags row for key — the write paths'
// pre-merge view (enabled state + stored config). exists reports whether a
// row exists at all: pgx.ErrNoRows maps to (registry default, empty config,
// exists=false, nil error) so callers can distinguish "unknown" (no Registry
// entry AND no row → 404) from "registered but never stored". Any other read
// error is returned — a silent exists=false here would let a PATCH merge
// onto an empty config and overwrite the stored row, destroying surface/
// phase/name data it never saw.
func (h *Handlers) extensionRow(ctx context.Context, key string) (enabled bool, cfg features.FlagConfig, exists bool, err error) {
	if h.DB == nil || h.DB.Reader == nil {
		return features.DefaultFor(key), features.FlagConfig{}, false, nil
	}

	row, err := gen.New(h.DB.Reader).GetFeatureFlagRowByKey(ctx, key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return features.DefaultFor(key), features.FlagConfig{}, false, nil
		}

		return false, features.FlagConfig{}, false, fmt.Errorf("reading feature flag row %s: %w", key, err)
	}

	return row.Enabled, decodeFlagConfig(row.Config), true, nil
}

// validateAudienceRefs resolves every group:<slug> and perm:<node> entry in
// the patched surface rules against the database — a typo'd slug or node
// would silently hide the feature behind an audience nobody matches.
func (h *Handlers) validateAudienceRefs(ctx context.Context, cfg features.FlagConfig) error {
	if h.DB == nil || h.DB.Reader == nil {
		return nil
	}

	groups := map[string]bool{}
	perms := map[string]bool{}

	for _, rule := range cfg.Surfaces {
		for _, entry := range rule.Audience {
			if slug, ok := strings.CutPrefix(entry, "group:"); ok {
				groups[slug] = true
			} else if node, ok := strings.CutPrefix(entry, "perm:"); ok {
				perms[node] = true
			}
		}
	}

	var missing []string

	check := func(query, col string, set map[string]bool, prefix string) error {
		if len(set) == 0 {
			return nil
		}

		vals := make([]string, 0, len(set))
		for v := range set {
			vals = append(vals, v)
		}

		rows, err := h.DB.Reader.Query(ctx, query, vals)
		if err != nil {
			return fmt.Errorf("validating audience %s: %w", col, err)
		}

		found := map[string]bool{}
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				rows.Close()
				return fmt.Errorf("validating audience %s: %w", col, err)
			}
			found[v] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("validating audience %s: %w", col, err)
		}
		rows.Close()

		for v := range set {
			if !found[v] {
				missing = append(missing, prefix+v)
			}
		}

		return nil
	}

	if err := check(`SELECT slug FROM user_groups WHERE slug = ANY($1)`, "groups", groups, "group:"); err != nil {
		return err
	}

	if err := check(`SELECT key FROM permissions WHERE key = ANY($1)`, "permissions", perms, "perm:"); err != nil {
		return err
	}

	if len(missing) > 0 {
		sort.Strings(missing)

		return fmt.Errorf("unknown audience entries: %s", strings.Join(missing, ", "))
	}

	return nil
}

// extensionKnown reports whether key is manageable through the extensions
// endpoints — a Registry key or an existing feature_flags row (the same
// union ListExtensions returns). A row read error propagates so write
// handlers fail the request (500) instead of merging onto an empty config
// or misreporting a stored-but-unreadable custom key as not found (404).
func (h *Handlers) extensionKnown(ctx context.Context, key string) (exists, enabled bool, cfg features.FlagConfig, err error) {
	storedEnabled, storedCfg, hasRow, err := h.extensionRow(ctx, key)
	if err != nil {
		return false, false, features.FlagConfig{}, err
	}

	if _, ok := features.RegistryLookup(key); !ok && !hasRow {
		return false, false, features.FlagConfig{}, nil
	}

	return true, storedEnabled, storedCfg, nil
}

// writeExtensionReadErr maps an extensionRow read failure to the standard
// error envelope — the row read is a precondition for every extension write.
func (h *Handlers) writeExtensionReadErr(w http.ResponseWriter, err error, key string) {
	slog.Error("control: extension row read failed", "error", err, "key", key)
	errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read extension")
}

// extensionPatchInput is the PATCH /control/extensions/{key} body. Every
// field is optional; only fields present in the body are merged into the
// stored FlagConfig (enabled merges into the row's enabled column).
type extensionPatchInput struct {
	Enabled           *bool                            `json:"enabled"`
	Version           *string                          `json:"version"` // accepted for back-compat; not persisted
	Phase             *string                          `json:"phase"`
	ShowOnRoadmap     *bool                            `json:"show_on_roadmap"`
	Surfaces          *map[string]features.SurfaceRule `json:"surfaces"`
	Name              *string                          `json:"name"`
	Description       *string                          `json:"description"`
	RolloutPercentage *int                             `json:"rollout_percentage"`
	Internal          *bool                            `json:"internal"`
}

// extensionPatchFields translates the PATCH body into (a) the jsonb patch
// object PatchFlag merges into feature_flags.config — keys are FlagConfig
// field names, so the wire's snake_case names map to the stored camelCase —
// and (b) a FlagConfig carrying only the provided fields for ValidateConfig
// (the checks are field-local, so validating the patch validates the write).
// Surfaces is a whole-map replace when present — the manage UI sends only
// touched/non-default surfaces. Rollout 1-100 is validated upstream.
func extensionPatchFields(input *extensionPatchInput) ([]byte, features.FlagConfig) {
	patch := map[string]any{}

	var cfg features.FlagConfig

	if input.Phase != nil {
		cfg.Phase = strings.TrimSpace(*input.Phase)
		patch["phase"] = cfg.Phase
	}

	if input.Internal != nil {
		cfg.Internal = *input.Internal
		patch["internal"] = cfg.Internal
	}

	if input.ShowOnRoadmap != nil {
		cfg.ShowOnRoadmap = *input.ShowOnRoadmap
		patch["showOnRoadmap"] = cfg.ShowOnRoadmap
	}

	if input.Surfaces != nil {
		cfg.Surfaces = *input.Surfaces
		patch["surfaces"] = cfg.Surfaces
	}

	if input.Name != nil {
		cfg.Name = strings.TrimSpace(*input.Name)
		patch["name"] = cfg.Name
	}

	if input.Description != nil {
		cfg.Description = strings.TrimSpace(*input.Description)
		patch["description"] = cfg.Description
	}

	if input.RolloutPercentage != nil {
		p := *input.RolloutPercentage
		if p < 0 {
			p = 0
		}

		if p > 100 {
			p = 100
		}

		cfg.RolloutPercentage = p
		patch["rolloutPercentage"] = p
	}

	raw, err := json.Marshal(patch)
	if err != nil {
		return []byte("{}"), cfg
	}

	return raw, cfg
}

// UpdateExtension handles PATCH /control/extensions/{key}. Fields present in
// the body are validated (ValidateConfig) then persisted via
// Checker.PatchFlag, which merges them into feature_flags.config inside the
// upsert — atomically, so concurrent PATCHes can't lose each other's fields —
// and invalidates the flag cache + fires the config-change hook. Absent
// fields keep their stored values. Unknown keys → 404 extension_not_found;
// invalid phase/surface values → 400 with the ConfigError's machine-readable
// code (invalid_phase, invalid_surface, invalid_surface_state,
// invalid_audience, invalid_geo).
func (h *Handlers) UpdateExtension(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Extension key is required")
		return
	}

	if h.FeatureChecker == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "flags_unavailable", "Feature flag store is not available")
		return
	}

	exists, _, _, err := h.extensionKnown(r.Context(), key)
	if err != nil {
		h.writeExtensionReadErr(w, err, key)
		return
	}

	if !exists {
		errpkg.WriteError(w, http.StatusNotFound, "extension_not_found", "Unknown extension or tool")
		return
	}

	var input extensionPatchInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// Rollout writes must be 1-100 — 0 is ambiguous with "unset" and is
	// not a real "off" state (disable the flag instead).
	if input.RolloutPercentage != nil && (*input.RolloutPercentage < 1 || *input.RolloutPercentage > 100) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_rollout", "rollout_percentage must be between 1 and 100 — disable the flag to turn it off")
		return
	}

	patch, patchCfg := extensionPatchFields(&input)

	// Phase/surface/audience/geo vocabularies are validated before the
	// write; ConfigError carries the machine-readable code (invalid_phase,
	// invalid_surface, …) emitted in the 400 error envelope.
	if err := features.ValidateConfig(patchCfg); err != nil {
		features.WriteConfigError(w, err)
		return
	}

	// Audience entries must resolve — a typo'd group slug or permission node
	// would silently hide the feature behind an audience nobody matches.
	if err := h.validateAudienceRefs(r.Context(), patchCfg); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_audience", err.Error())
		return
	}

	// Uncached pre-write snapshot doubles as the audit row's "before" image.
	before := h.snapshotFlag(r.Context(), key)
	updatedBy := authUserFromContext(r)

	// PatchFlag merges inside the upsert — concurrent PATCHes can't lose each
	// other's fields the way a read-modify-write would. The RETURNING values
	// are the persisted state, used for the audit and response.
	enabled, merged, err := h.FeatureChecker.PatchFlag(r.Context(), key, input.Enabled, patch, updatedBy)
	if err != nil {
		slog.Error("control: update extension", "error", err, "key", key)
		errpkg.HandleServiceError(w, err)

		return
	}

	h.fireFlagEnabled(r.Context(), key, before.Enabled, enabled)
	h.auditFlagWrite(r.Context(), updatedBy, clientip.FromRequest(r), "feature_flag.update", key,
		before, &flagAuditSnapshot{Enabled: enabled, Config: merged})

	data := map[string]any{"key": key, "status": "updated", "enabled": enabled}

	// Spec §6.3: show_on_roadmap=true while roadmap.enabled resolves off is
	// allowed, but the response carries the warning so the UI can flag it.
	if merged.ShowOnRoadmap && !h.FeatureChecker.IsEnabled(r.Context(), "roadmap.enabled") {
		data["warning"] = "roadmap_disabled"
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// EnableExtension handles POST /control/extensions/{key}/enable.
func (h *Handlers) EnableExtension(w http.ResponseWriter, r *http.Request) {
	h.setExtensionEnabled(w, r, true)
}

// DisableExtension handles POST /control/extensions/{key}/disable.
func (h *Handlers) DisableExtension(w http.ResponseWriter, r *http.Request) {
	h.setExtensionEnabled(w, r, false)
}

// DeleteExtension handles DELETE /control/extensions/{key}. Removes the
// stored feature_flags row — the key reverts to its Registry default (or
// disappears entirely for admin-created "Custom" keys). The config-change
// hook fires with an empty config so roadmap sync unlinks the item.
func (h *Handlers) DeleteExtension(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Extension key is required")
		return
	}

	if h.FeatureChecker == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "flags_unavailable", "Feature flag store is not available")
		return
	}

	exists, _, _, err := h.extensionKnown(r.Context(), key)
	if err != nil {
		h.writeExtensionReadErr(w, err, key)
		return
	}

	if !exists {
		errpkg.WriteError(w, http.StatusNotFound, "extension_not_found", "Unknown extension or tool")
		return
	}

	before := h.snapshotFlag(r.Context(), key)
	updatedBy := authUserFromContext(r)

	if err := h.FeatureChecker.DeleteFlag(r.Context(), key); err != nil {
		slog.Error("control: delete extension flag", "error", err, "key", key)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete extension")

		return
	}

	h.auditFlagWrite(r.Context(), updatedBy, clientip.FromRequest(r), "feature_flag.delete", key,
		before, nil)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "status": "deleted"}})
}

// setExtensionEnabled backs the enable/disable endpoints. The flag key is
// validated against the Registry ∪ feature_rows union (404 for unknown
// keys), written via Checker.SetEnabled (which invalidates the flag cache
// and fires the config-change hook), and audit-logged.
func (h *Handlers) setExtensionEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	key := chi.URLParam(r, "key")
	if key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Extension key is required")
		return
	}

	if h.FeatureChecker == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "flags_unavailable", "Feature flag store is not available")
		return
	}

	exists, _, _, err := h.extensionKnown(r.Context(), key)
	if err != nil {
		h.writeExtensionReadErr(w, err, key)
		return
	}

	if !exists {
		errpkg.WriteError(w, http.StatusNotFound, "extension_not_found", "Unknown extension or tool")
		return
	}

	before := h.snapshotFlag(r.Context(), key)
	updatedBy := authUserFromContext(r)

	if err := h.FeatureChecker.SetEnabled(r.Context(), key, enabled, updatedBy); err != nil {
		slog.Error("control: set extension enabled", "error", err, "key", key)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update extension")

		return
	}

	status := "disabled"
	if enabled {
		status = "enabled"
	}

	h.fireFlagEnabled(r.Context(), key, before.Enabled, enabled)
	h.auditFlagWrite(r.Context(), updatedBy, clientip.FromRequest(r), "feature_flag.update", key,
		before, &flagAuditSnapshot{Enabled: enabled, Config: before.Config})

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key, "status": status}})
}

// UpdateExtensionRollout updates the rollout percentage for a feature flag
// via the extensions catalog. The percentage is clamped to 0-100. A value of
// 100 means fully on (no hash bucketing), 0 means off, and 1-99 enables
// deterministic per-platform gradual rollout (BL-013 / Section 42).
func (h *Handlers) UpdateExtensionRollout(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Extension key is required")
		return
	}

	if h.FeatureChecker == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "flags_unavailable", "Feature flag store is not available")
		return
	}

	exists, _, _, err := h.extensionKnown(r.Context(), key)
	if err != nil {
		h.writeExtensionReadErr(w, err, key)
		return
	}

	if !exists {
		errpkg.WriteError(w, http.StatusNotFound, "extension_not_found", "Unknown extension or tool")
		return
	}

	var input struct {
		RolloutPercentage *int `json:"rollout_percentage"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.RolloutPercentage == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_rollout", "rollout_percentage is required")
		return
	}

	percentage := *input.RolloutPercentage
	if percentage < 1 || percentage > 100 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_rollout", "rollout_percentage must be between 1 and 100 — disable the flag to turn it off")
		return
	}

	before := h.snapshotFlag(r.Context(), key)
	updatedBy := authUserFromContext(r)

	if err := h.FeatureChecker.SetRolloutPercentage(r.Context(), key, percentage, updatedBy); err != nil {
		slog.Error("control: update rollout", "error", err, "key", key)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update rollout percentage")

		return
	}

	afterCfg := before.Config
	afterCfg.RolloutPercentage = percentage
	h.auditFlagWrite(r.Context(), updatedBy, clientip.FromRequest(r), "feature_flag.update", key,
		before, &flagAuditSnapshot{Enabled: before.Enabled, Config: afterCfg})

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"key": key, "rollout_percentage": percentage}})
}

// flagAuditSnapshot is the before/after image stored in an audit_logs row's
// metadata for a flag write — mirrors flagAuditSnapshot in the features
// package (unexported there, so control keeps its own copy).
type flagAuditSnapshot struct {
	Enabled bool                `json:"enabled"`
	Config  features.FlagConfig `json:"config"`
}

// snapshotFlag reads the flag's current enabled state + stored config for an
// audit row's "before" image. QueryEnabled bypasses the L1/L2 caches so a
// stale cached value never misstates the pre-write state. Returns zero
// values when the checker or DB isn't wired or the row doesn't exist.
func (h *Handlers) snapshotFlag(ctx context.Context, key string) *flagAuditSnapshot {
	if h.FeatureChecker == nil {
		return &flagAuditSnapshot{}
	}

	cfg, _ := h.FeatureChecker.GetFlagConfig(ctx, key)

	return &flagAuditSnapshot{Enabled: h.FeatureChecker.QueryEnabled(ctx, key), Config: cfg}
}

// auditFlagWrite appends an audit_logs row for an extension flag write,
// following the same insert pattern as the features package's admin flag
// handlers. target_id stays NULL — flag keys aren't UUIDs — so the key rides
// in metadata alongside the before/after snapshots. A nil actor or empty IP
// is written as NULL. Audit failure is logged but never fails the request:
// the flag write already committed, so returning an error would invite a
// retry of a succeeded mutation.
func (h *Handlers) auditFlagWrite(ctx context.Context, actor pgtype.UUID, ip, action, key string, before, after *flagAuditSnapshot) {
	if h.DB == nil || h.DB.Writer == nil {
		return
	}

	metaJSON, err := json.Marshal(map[string]any{
		"key":    key,
		"before": before,
		"after":  after,
	})
	if err != nil {
		slog.Error("control: audit metadata marshal failed", "action", action, "key", key, "error", err)
		return
	}

	var actorArg any
	if actor.Valid {
		actorArg = actor
	}

	// Pass nil for empty ip so PostgreSQL doesn't reject ''::inet.
	var ipArg any
	if ip != "" {
		ipArg = ip
	}

	if _, err := h.DB.Writer.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, action_type, target_type, metadata, ip_address)
		VALUES ($1, $2, 'feature_flag', $3::jsonb, $4::inet)
	`, actorArg, action, metaJSON, ipArg); err != nil {
		slog.Error("control: audit log insert failed", "action", action, "key", key, "error", err)
	}
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
	return valkey.MustKey("backup_configs:%s", pgtype.UUID{}.String())
}

// ListBackupConfigs is an exported method.
func (h *Handlers) ListBackupConfigs(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	raw, err := h.Valkey.Get(r.Context(), backupConfigsKey(r.Context())).Result()
	if err != nil || raw == "" {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	var configs []backupConfigEntry
	if json.Unmarshal([]byte(raw), &configs) != nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": configs})
}

// ListBackupTargets handles GET /control/backup-targets — returns the list of
// components that can be included in a backup.
func (h *Handlers) ListBackupTargets(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": supportedBackupTargets})
}

// CreateBackupConfig is an exported method.
func (h *Handlers) CreateBackupConfig(w http.ResponseWriter, r *http.Request) {
	if h.Valkey == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_valkey", "Valkey not available")
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" || input.Schedule == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "name and schedule are required")
		return
	}
	// Validate backup targets.
	validTargets := make(map[string]bool)
	for _, t := range supportedBackupTargets {
		validTargets[t["key"]] = true
	}

	for _, t := range input.WhatToBackup {
		if !validTargets[t] {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_target", "Unknown backup target: "+t)
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
		errpkg.WriteError(w, http.StatusInternalServerError, "save_error", "Failed to save backup config")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": entry})
}

// UpdateBackupConfig is an exported method.
func (h *Handlers) UpdateBackupConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_id", "id is required")
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
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
				errpkg.WriteError(w, http.StatusInternalServerError, "save_error", "Failed to update backup config")
				return
			}

			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": configs[i]})

			return
		}
	}

	errpkg.WriteError(w, http.StatusNotFound, "not_found", "Backup config not found")
}

func (h *Handlers) DeleteBackupConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_id", "id is required")
		return
	}

	configs := h.loadBackupConfigs(r)
	for i, c := range configs {
		if c.ID == id {
			configs = append(configs[:i], configs[i+1:]...)
			if err := h.saveBackupConfigs(r, configs); err != nil {
				errpkg.WriteError(w, http.StatusInternalServerError, "save_error", "Failed to delete backup config")
				return
			}

			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})

			return
		}
	}

	errpkg.WriteError(w, http.StatusNotFound, "not_found", "Backup config not found")
}

// TriggerBackup is an exported method.
func (h *Handlers) TriggerBackup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_id", "id is required")
		return
	}

	if h.Client == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "queue_unavailable", "Task queue not available")
		return
	}

	configs := h.loadBackupConfigs(r)
	for i := range configs {
		if configs[i].ID == id {
			configs[i].LastRunAt = time.Now().UTC().Format(time.RFC3339)
			if err := h.saveBackupConfigs(r, configs); err != nil {
				slog.Error("control: trigger backup update last_run", "error", err)
			}

			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"id": id, "status": "triggered"}})

			return
		}
	}

	errpkg.WriteError(w, http.StatusNotFound, "not_found", "Backup config not found")
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
		return errors.New("valkey not available")
	}

	data, err := json.Marshal(configs)
	if err != nil {
		return err
	}

	return h.Valkey.Set(r.Context(), backupConfigsKey(r.Context()), string(data), 0).Err()
}

// humanizeBytes returns a human-readable string for a byte count.
func humanizeBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}

	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
