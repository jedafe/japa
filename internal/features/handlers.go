package features

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers holds dependencies for feature flag HTTP handlers.
type Handlers struct {
	checker *Checker
}

// NewHandlers creates a new Handlers with the given Checker.
func NewHandlers(checker *Checker) *Handlers {
	return &Handlers{checker: checker}
}

// HandleGetFeatureFlags handles GET /api/v1/feature-flags.
// Returns all feature flags and their enabled state for the current tenant.
// This endpoint is public (no auth required) because the frontend layout
// needs flags before the user logs in to render navigation correctly.
func (h *Handlers) HandleGetFeatureFlags(w http.ResponseWriter, r *http.Request) {

	flags := h.checker.GetAllEnabled(r.Context(), pgtype.UUID{})
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": flags})
}

// HandleSetFeatureFlag handles PATCH /admin/feature-flags/{flag_name}.
// Toggles a feature flag's enabled state for the current tenant.
// Protected by RBAC permission "features.manage".
func (h *Handlers) HandleSetFeatureFlag(w http.ResponseWriter, r *http.Request) {

	flagName := chi.URLParam(r, "flag_name")
	if flagName == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_flag_name", "flag_name is required")
		return
	}

	var input struct {
		Enabled bool `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	if err := h.checker.SetEnabled(r.Context(), pgtype.UUID{}, flagName, input.Enabled, user.UserID); err != nil {
		slog.Error("features: set enabled failed", "flag", flagName, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update feature flag")

		return
	}

	// STD-003: clear only the affected tenant's cache, not all tenants.
	h.checker.ClearCache()

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"key":       flagName,
			"isEnabled": input.Enabled,
		},
	})
}

// FeatureFlag is the admin-facing representation of a tenant feature flag.
// The storage layer only persists a flag_name → enabled mapping, so the
// richer fields (name, description, environment, rolloutPercentage) are
// derived with sensible defaults for the admin UI.
type FeatureFlag struct {
	ID                string `json:"id"`
	Key               string `json:"key"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	IsEnabled         bool   `json:"isEnabled"`
	Environment       string `json:"environment"`
	RolloutPercentage int    `json:"rolloutPercentage"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
}

// ListFeatureFlags handles GET /api/v1/admin/feature-flags.
// Returns the tenant's feature flags as an array (the public GET /feature-flags
// returns a map). Gated by admin.feature_flags.manage at the router so the admin
// SPA can render a proper list/table. It merges the static Registry keys with
// any tenant rows (including admin-created keys) so the list is always complete.
func (h *Handlers) ListFeatureFlags(w http.ResponseWriter, r *http.Request) {

	rows, err := h.checker.ListFlagRows(r.Context(), pgtype.UUID{})
	if err != nil {
		slog.Error("features: list rows failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list feature flags")

		return
	}

	// Seed every registry key with its default so it always appears, then
	// overlay any stored row (which carries the real enabled state + config).
	byKey := make(map[string]FeatureFlag, len(Registry))
	for _, f := range Registry {
		byKey[f.Key] = FeatureFlag{
			ID:                f.Key,
			Key:               f.Key,
			Name:              f.Label,
			IsEnabled:         f.Default,
			Environment:       "production",
			RolloutPercentage: 100,
		}
	}

	for _, row := range rows {
		flag := byKey[row.Key]
		flag.ID = row.Key
		flag.Key = row.Key
		flag.IsEnabled = row.Enabled
		flag.UpdatedAt = row.UpdatedAt.Format(time.RFC3339)
		applyFlagConfig(&flag, row.Config)
		byKey[row.Key] = flag
	}

	flags := make([]FeatureFlag, 0, len(byKey))
	for _, f := range byKey {
		flags = append(flags, f)
	}

	sort.Slice(flags, func(i, j int) bool { return flags[i].Key < flags[j].Key })

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": flags})
}

// applyFlagConfig overlays the jsonb config blob onto a FeatureFlag, falling
// back to the existing values when the blob is absent or malformed.
func applyFlagConfig(flag *FeatureFlag, raw []byte) {
	if len(raw) == 0 {
		return
	}

	var cfg FlagConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return
	}

	if cfg.Name != "" {
		flag.Name = cfg.Name
	}

	if cfg.Description != "" {
		flag.Description = cfg.Description
	}

	if cfg.Environment != "" {
		flag.Environment = cfg.Environment
	}

	if cfg.RolloutPercentage != 0 {
		flag.RolloutPercentage = cfg.RolloutPercentage
	}
}

// CreateFeatureFlag handles POST /api/v1/admin/feature-flags.
// Creates a tenant feature flag (registry key or new admin key) with an initial
// enabled state and metadata. Gated by admin.feature_flags.manage.
func (h *Handlers) CreateFeatureFlag(w http.ResponseWriter, r *http.Request) {

	var input FeatureFlagInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	key := strings.TrimSpace(input.Key)
	if key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "key is required")
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	cfg := FlagConfig{
		Name:              strings.TrimSpace(input.Name),
		Description:       strings.TrimSpace(input.Description),
		Environment:       strings.TrimSpace(input.Environment),
		RolloutPercentage: input.RolloutPercentage,
	}
	if cfg.Name == "" {
		cfg.Name = humanizeFlagName(key)
	}

	if cfg.Environment == "" {
		cfg.Environment = "production"
	}

	if cfg.RolloutPercentage == 0 {
		cfg.RolloutPercentage = 100
	}

	if err := h.checker.UpdateFlag(r.Context(), pgtype.UUID{}, key, input.IsEnabled, cfg, user.UserID); err != nil {
		slog.Error("features: create failed", "key", key, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create feature flag")

		return
	}

	h.checker.ClearCache()
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": FeatureFlag{
			ID:                key,
			Key:               key,
			Name:              cfg.Name,
			Description:       cfg.Description,
			IsEnabled:         input.IsEnabled,
			Environment:       cfg.Environment,
			RolloutPercentage: cfg.RolloutPercentage,
		},
	})
}

// UpdateFeatureFlag handles PUT /api/v1/admin/feature-flags/{flag_name}.
// Persists the enabled state plus richer admin metadata (name/description/
// environment/rollout) into the config jsonb column. Gated by
// admin.feature_flags.manage.
func (h *Handlers) UpdateFeatureFlag(w http.ResponseWriter, r *http.Request) {

	flagName := chi.URLParam(r, "flag_name")
	if flagName == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_flag_name", "flag_name is required")
		return
	}

	var input FeatureFlagInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	cfg := FlagConfig{
		Name:              strings.TrimSpace(input.Name),
		Description:       strings.TrimSpace(input.Description),
		Environment:       strings.TrimSpace(input.Environment),
		RolloutPercentage: input.RolloutPercentage,
	}
	if cfg.Name == "" {
		cfg.Name = humanizeFlagName(flagName)
	}

	if cfg.Environment == "" {
		cfg.Environment = "production"
	}

	if cfg.RolloutPercentage == 0 {
		cfg.RolloutPercentage = 100
	}

	if err := h.checker.UpdateFlag(r.Context(), pgtype.UUID{}, flagName, input.IsEnabled, cfg, user.UserID); err != nil {
		slog.Error("features: update failed", "key", flagName, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update feature flag")

		return
	}

	h.checker.ClearCache()
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": FeatureFlag{
			ID:                flagName,
			Key:               flagName,
			Name:              cfg.Name,
			Description:       cfg.Description,
			IsEnabled:         input.IsEnabled,
			Environment:       cfg.Environment,
			RolloutPercentage: cfg.RolloutPercentage,
		},
	})
}

// DeleteFeatureFlag handles DELETE /api/v1/admin/feature-flags/{flag_name}.
// Removes the tenant's override for the flag; afterwards it reverts to its
// registry Default. Gated by admin.feature_flags.manage.
func (h *Handlers) DeleteFeatureFlag(w http.ResponseWriter, r *http.Request) {

	flagName := chi.URLParam(r, "flag_name")
	if flagName == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_flag_name", "flag_name is required")
		return
	}

	if err := h.checker.DeleteFlag(r.Context(), pgtype.UUID{}, flagName); err != nil {
		slog.Error("features: delete failed", "key", flagName, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete feature flag")

		return
	}

	h.checker.ClearCache()
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"key": flagName, "deleted": true},
	})
}

// FeatureFlagInput is the request body shared by Create/Update handlers.
type FeatureFlagInput struct {
	Name              string `json:"name"`
	Key               string `json:"key"`
	Description       string `json:"description"`
	IsEnabled         bool   `json:"isEnabled"`
	Environment       string `json:"environment"`
	RolloutPercentage int    `json:"rolloutPercentage"`
}

// humanizeFlagName turns a dotted/snake_case flag key into a readable Title
// Case label. Splits on both "." and "_" so keys like "feature.blog" and
// "forum.ama.enabled" produce "Feature Blog" and "Forum Ama Enabled".
func humanizeFlagName(name string) string {
	name = strings.ReplaceAll(name, ".", "_")

	parts := strings.Split(name, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}

		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}

	return strings.Join(parts, " ")
}
