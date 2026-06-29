// Package admin — HTTP handlers for admin-only platform management endpoints.
//
// Settings handlers manage the system_settings table, providing a structured
// API for reading and writing platform-wide configuration organized by category.
package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/tenant"
)

// SettingDef defines the metadata for a single setting within a category.
type SettingDef struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Type    string   `json:"type"` // "text", "number", "toggle", "select"
	Value   any      `json:"value,omitempty"`
	Options []string `json:"options,omitempty"`
}

// SettingCategory groups related settings with display metadata.
type SettingCategory struct {
	Key      string      `json:"key"`
	Label    string      `json:"label"`
	Settings []SettingDef `json:"settings"`
}

// SettingValue holds a raw JSONB value read from the database.
type SettingValue struct {
	Key   string
	Value any
}

// Handlers holds dependencies for admin HTTP handlers.
type Handlers struct {
	DB *db.DB
}

// NewHandlers creates Handlers with the given database pool.
func NewHandlers(database *db.DB) *Handlers {
	return &Handlers{DB: database}
}

// defaultSettings defines every available setting with its metadata.
// The first segment of the dot-notation key (e.g. "general" from
// "general.site_name") determines the category.
var defaultSettings = map[string]SettingCategory{
	"general": {
		Key:   "general",
		Label: "General",
		Settings: []SettingDef{
			{Key: "site_name", Label: "Site Name", Type: "text"},
			{Key: "site_description", Label: "Site Description", Type: "text"},
			{Key: "maintenance_mode", Label: "Maintenance Mode", Type: "toggle"},
			{Key: "registration_open", Label: "Open Registration", Type: "toggle"},
		},
	},
	"features": {
		Key:   "features",
		Label: "Features",
		Settings: []SettingDef{
			{Key: "blog_enabled", Label: "Blog", Type: "toggle"},
			{Key: "forum_enabled", Label: "Forum", Type: "toggle"},
			{Key: "reviews_enabled", Label: "Reviews", Type: "toggle"},
			{Key: "comparisons_enabled", Label: "Comparisons", Type: "toggle"},
			{Key: "advertising_enabled", Label: "Advertising", Type: "toggle"},
		},
	},
	"email": {
		Key:   "email",
		Label: "Email",
		Settings: []SettingDef{
			{Key: "smtp_host", Label: "SMTP Host", Type: "text"},
			{Key: "smtp_port", Label: "SMTP Port", Type: "number"},
			{Key: "from_address", Label: "From Address", Type: "text"},
		},
	},
	"security": {
		Key:   "security",
		Label: "Security",
		Settings: []SettingDef{
			{Key: "max_login_attempts", Label: "Max Login Attempts", Type: "number"},
			{Key: "lockout_duration_minutes", Label: "Lockout Duration (min)", Type: "number"},
			{Key: "require_email_verification", Label: "Require Email Verification", Type: "toggle"},
		},
	},
	"appearance": {
		Key:   "appearance",
		Label: "Appearance",
		Settings: []SettingDef{
			{Key: "primary_color", Label: "Primary Color", Type: "text"},
			{Key: "logo_url", Label: "Logo URL", Type: "text"},
			{Key: "favicon_url", Label: "Favicon URL", Type: "text"},
			{Key: "dark_mode_default", Label: "Dark Mode Default", Type: "toggle"},
		},
	},
	"users": {
		Key:   "users",
		Label: "Users",
		Settings: []SettingDef{
			{Key: "allow_social_login", Label: "Allow Social Login", Type: "toggle"},
			{Key: "require_profile_completion", Label: "Require Profile Completion", Type: "toggle"},
			{Key: "max_avatar_size_mb", Label: "Max Avatar Size (MB)", Type: "number"},
			{Key: "allowed_avatar_types", Label: "Allowed Avatar Types", Type: "text"},
		},
	},
	"content": {
		Key:   "content",
		Label: "Content",
		Settings: []SettingDef{
			{Key: "posts_per_page", Label: "Posts Per Page", Type: "number"},
			{Key: "allow_anonymous_posts", Label: "Allow Anonymous Posts", Type: "toggle"},
			{Key: "require_content_review", Label: "Require Content Review", Type: "toggle"},
			{Key: "max_upload_size_mb", Label: "Max Upload Size (MB)", Type: "number"},
			{Key: "allowed_upload_types", Label: "Allowed Upload Types", Type: "text"},
		},
	},
	"payment": {
		Key:   "payment",
		Label: "Payment",
		Settings: []SettingDef{
			{Key: "currency", Label: "Currency", Type: "select", Options: []string{"NGN", "USD", "GBP", "EUR"}},
			{Key: "paystack_public_key", Label: "Paystack Public Key", Type: "text"},
			{Key: "paystack_secret_key", Label: "Paystack Secret Key", Type: "text"},
			{Key: "stripe_public_key", Label: "Stripe Public Key", Type: "text"},
			{Key: "stripe_secret_key", Label: "Stripe Secret Key", Type: "text"},
		},
	},
	"media": {
		Key:   "media",
		Label: "Media",
		Settings: []SettingDef{
			{Key: "imgproxy_enabled", Label: "imgproxy Enabled", Type: "toggle"},
			{Key: "imgproxy_key", Label: "imgproxy Key", Type: "text"},
			{Key: "imgproxy_salt", Label: "imgproxy Salt", Type: "text"},
			{Key: "max_image_width", Label: "Max Image Width (px)", Type: "number"},
			{Key: "max_image_height", Label: "Max Image Height (px)", Type: "number"},
		},
	},
	"seo": {
		Key:   "seo",
		Label: "SEO",
		Settings: []SettingDef{
			{Key: "meta_title_template", Label: "Meta Title Template", Type: "text"},
			{Key: "meta_description_template", Label: "Meta Description Template", Type: "text"},
			{Key: "google_analytics_id", Label: "Google Analytics ID", Type: "text"},
			{Key: "enable_sitemap", Label: "Enable Sitemap", Type: "toggle"},
			{Key: "enable_robots_txt", Label: "Enable robots.txt", Type: "toggle"},
		},
	},
	"integrations": {
		Key:   "integrations",
		Label: "Integrations",
		Settings: []SettingDef{
			{Key: "meilisearch_url", Label: "Meilisearch URL", Type: "text"},
			{Key: "meilisearch_key", Label: "Meilisearch Key", Type: "text"},
			{Key: "honcho_enabled", Label: "Honcho Enabled", Type: "toggle"},
			{Key: "honcho_url", Label: "Honcho URL", Type: "text"},
		},
	},
	"ai": {
		Key:   "ai",
		Label: "AI",
		Settings: []SettingDef{
			{Key: "ai_assist_enabled", Label: "AI Assist Enabled", Type: "toggle"},
			{Key: "ai_model", Label: "AI Model", Type: "select", Options: []string{"gpt-4", "gpt-3.5-turbo", "claude-3"}},
			{Key: "ai_max_tokens", Label: "AI Max Tokens", Type: "number"},
			{Key: "ai_content_moderation", Label: "AI Content Moderation", Type: "toggle"},
		},
	},
	"advanced": {
		Key:   "advanced",
		Label: "Advanced",
		Settings: []SettingDef{
			{Key: "debug_mode", Label: "Debug Mode", Type: "toggle"},
			{Key: "log_level", Label: "Log Level", Type: "select", Options: []string{"debug", "info", "warn", "error"}},
			{Key: "cache_ttl_seconds", Label: "Cache TTL (seconds)", Type: "number"},
			{Key: "rate_limit_per_minute", Label: "Rate Limit (per minute)", Type: "number"},
		},
	},
}

// settingCategoryOrder controls the response order sent to the frontend.
var settingCategoryOrder = []string{
	"general", "features", "email", "security",
	"appearance", "users", "content", "payment",
	"media", "seo", "integrations", "ai", "advanced",
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// requireTenant resolves the tenant from the request context.
func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

// ---------------------------------------------------------------------------
// ListSettings — GET /admin/settings
// ---------------------------------------------------------------------------

// ListSettings returns all platform settings for the current tenant,
// organized by category. The response shape matches the frontend's
// expected format: an array of categories, each containing an array
// of settings with their current values.
func (h *Handlers) ListSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	ctx := r.Context()

	// Fetch all settings for this tenant from the database.
	rows, err := h.DB.Reader.Query(ctx,
		`SELECT key, value FROM system_settings WHERE tenant_id = $1`,
		tenantID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to load settings")
		return
	}
	defer rows.Close()

	// Build a map of stored values keyed by the short key (after the dot).
	stored := make(map[string]any)
	for rows.Next() {
		var fullKey string
		var value any
		if err := rows.Scan(&fullKey, &value); err != nil {
			continue
		}
		// The stored key is dot-notation (e.g. "general.site_name");
		// the short key is the part after the first dot.
		parts := strings.SplitN(fullKey, ".", 2)
		if len(parts) == 2 {
			stored[parts[1]] = value
		}
	}

	// Assemble the response by iterating over defined categories in order,
	// merging stored values with the schema defaults.
	result := make([]SettingCategory, 0, len(settingCategoryOrder))
	for _, catKey := range settingCategoryOrder {
		def, ok := defaultSettings[catKey]
		if !ok {
			continue
		}

		settings := make([]SettingDef, len(def.Settings))
		for i, s := range def.Settings {
			settings[i] = s
			if val, exists := stored[s.Key]; exists {
				settings[i].Value = val
			}
		}

		result = append(result, SettingCategory{
			Key:      def.Key,
			Label:    def.Label,
			Settings: settings,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// ---------------------------------------------------------------------------
// UpdateSettings — PUT /admin/settings/{category}
// ---------------------------------------------------------------------------

// UpdateSettings saves settings for a single category. The request body
// contains a flat map of key-value pairs (e.g. {"site_name": "New Name"}).
// Each pair is upserted into the system_settings table using the full
// dot-notation key (category.setting_key).
func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	category := chi.URLParam(r, "category")
	if category == "" {
		writeError(w, http.StatusBadRequest, "missing_category", "Category is required")
		return
	}

	// Validate that the category exists in our schema.
	def, ok := defaultSettings[category]
	if !ok {
		writeError(w, http.StatusNotFound, "invalid_category", fmt.Sprintf("Unknown settings category: %s", category))
		return
	}

	// Parse the request body as key-value pairs.
	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if len(updates) == 0 {
		writeError(w, http.StatusBadRequest, "empty_body", "No settings provided")
		return
	}

	// Build a set of valid keys for this category so we can reject unknown keys.
	validKeys := make(map[string]bool, len(def.Settings))
	for _, s := range def.Settings {
		validKeys[s.Key] = true
	}

	ctx := r.Context()
	tx, err := h.DB.Writer.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tx_error", "Failed to start transaction")
		return
	}
	defer tx.Rollback(ctx)

	// Upsert each setting. Only keys defined in the schema are accepted.
	for key, value := range updates {
		if !validKeys[key] {
			writeError(w, http.StatusBadRequest, "invalid_key", fmt.Sprintf("Unknown setting key: %s", key))
			return
		}

		fullKey := category + "." + key
		valueJSON, err := json.Marshal(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Cannot serialize value for key: %s", key))
			return
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO system_settings (tenant_id, key, value, created_at, updated_at)
			VALUES ($1, $2, $3::jsonb, now(), now())
			ON CONFLICT (tenant_id, key)
			DO UPDATE SET value = EXCLUDED.value, updated_at = now()
		`, tenantID, fullKey, string(valueJSON))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", fmt.Sprintf("Failed to save setting: %s", key))
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "commit_error", "Failed to save settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"status":   "saved",
			"category": category,
		},
	})
}
