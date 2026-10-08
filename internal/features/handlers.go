package features

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers holds dependencies for feature flag HTTP handlers.
type Handlers struct {
	checker *Checker
	// onEnabled fires after a flag transitions off→on via the admin write
	// endpoints (§80: wiki.enabled activation seeds stub pages). Optional.
	onEnabled func(ctx context.Context, flagName string)
}

// NewHandlers creates a new Handlers with the given Checker.
func NewHandlers(checker *Checker) *Handlers {
	return &Handlers{checker: checker}
}

// SetOnFlagEnabled registers a callback invoked when a flag transitions to
// enabled through HandleSetFeatureFlag/UpdateFeatureFlag/CreateFeatureFlag.
// Wired in server.go to trigger the wiki:seed_stub_pages fan-out (§80).
func (h *Handlers) SetOnFlagEnabled(fn func(ctx context.Context, flagName string)) {
	h.onEnabled = fn
}

// SetOnConfigChanged registers the post-write config hook. It delegates to
// the Checker so the same hook also fires for write paths that never touch
// these handlers (/control/extensions calling SetEnabled/SetRolloutPercentage
// directly).
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §4.4.
func (h *Handlers) SetOnConfigChanged(fn func(ctx context.Context, key string, cfg FlagConfig)) {
	h.checker.SetOnConfigChanged(fn)
}

// flagAuditSnapshot is the before/after image stored in an audit_logs row's
// metadata for a feature flag write — the resolved enabled state plus the
// full stored config blob.
type flagAuditSnapshot struct {
	Enabled bool       `json:"enabled"`
	Config  FlagConfig `json:"config"`
}

// snapshotFlag reads the flag's current enabled state + stored config for
// an audit row's "before" image. Returns zero values when the DB isn't
// wired or the row doesn't exist.
func (h *Handlers) snapshotFlag(ctx context.Context, key string) *flagAuditSnapshot {
	cfg, _ := h.checker.GetFlagConfig(ctx, key)

	return &flagAuditSnapshot{Enabled: h.checker.QueryEnabled(ctx, key), Config: cfg}
}

// auditFlagWrite appends an audit_logs row for a flag admin write, following
// the insert pattern used by the impersonation service (Rule: writes must be
// auditable). target_id stays NULL — flag keys aren't UUIDs — so the key
// rides in metadata alongside the before/after config snapshots. A nil actor
// or empty IP is written as NULL. Audit failure is logged but never fails
// the request: the flag write already committed, so returning an error would
// invite a retry of a succeeded mutation.
func (h *Handlers) auditFlagWrite(ctx context.Context, actor pgtype.UUID, ip, action, key string, before, after *flagAuditSnapshot) {
	if h.checker == nil || h.checker.db == nil {
		return
	}

	metaJSON, err := json.Marshal(map[string]any{
		"key":    key,
		"before": before,
		"after":  after,
	})
	if err != nil {
		slog.Error("features: audit metadata marshal failed", "action", action, "key", key, "error", err)
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

	if _, err := h.checker.db.Writer.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, action_type, target_type, metadata, ip_address)
		VALUES ($1, $2, 'feature_flag', $3::jsonb, $4::inet)
	`, actorArg, action, metaJSON, ipArg); err != nil {
		slog.Error("features: audit log insert failed", "action", action, "key", key, "error", err)
	}
}

// fireOnEnabled runs the activation hook when a write transitioned a flag
// off→on. wasEnabled must be read before the mutation. Rule 8 — the hook
// fans out one task per country space, so it must not block the request;
// the goroutine detaches from the request context (which dies on response)
// via context.WithoutCancel.
func (h *Handlers) fireOnEnabled(ctx context.Context, flagName string, wasEnabled, nowEnabled bool) {
	if h.onEnabled == nil || wasEnabled || !nowEnabled {
		return
	}
	go h.onEnabled(context.WithoutCancel(ctx), flagName)
}

// HandleGetFeatureFlags handles GET /api/v1/feature-flags.
// Returns all feature flags as key → Enabled && Visible for the caller's
// scope (surface header, geo context, resolved audience — spec §5.3).
// This endpoint is public (no auth required) because the frontend layout
// needs flags before the user logs in to render navigation correctly;
// OptionalAuth populates identity when a session exists, so signed-in
// callers still get their audience/geo overlays.
func (h *Handlers) HandleGetFeatureFlags(w http.ResponseWriter, r *http.Request) {

	sc := h.checker.BuildScope(r, SurfaceFrom(r))
	flags := h.checker.EvalAll(r.Context(), sc)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": flags})
}

// FeatureFlagDetail is the public per-flag projection returned by
// GET /api/v1/feature-flags/details. It deliberately exposes only the
// fields a surface needs to render state badges ("Coming soon", "Beta",
// rollout %). surfaces/audience/geo rules are internal config — group
// slugs, permission nodes, and geo policy are never exposed publicly;
// the full rule set is available only via /api/v1/control/extensions.
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §5.3.
type FeatureFlagDetail struct {
	Key               string `json:"key"`
	Enabled           bool   `json:"enabled"`
	Phase             string `json:"phase"`
	RolloutPercentage int    `json:"rolloutPercentage"`
	Visible           bool   `json:"visible"`
}

// HandleGetFeatureFlagDetails handles GET /api/v1/feature-flags/details.
// Same public/no-auth posture as the flags map endpoint — it reveals only
// what an attacker could already probe via route 404s. Returns the
// Registry + DB merge (same merge shape as ListFeatureFlags) with each
// entry's enabled/visible evaluated for the caller's scope.
func (h *Handlers) HandleGetFeatureFlagDetails(w http.ResponseWriter, r *http.Request) {

	sc := h.checker.BuildScope(r, SurfaceFrom(r))

	// Seed every registry key with its default, then overlay stored rows via
	// the shared jc:flags:_all aggregate — no per-request Postgres hit.
	merged := make(map[string]FlagRow, len(Registry))
	for _, f := range Registry {
		merged[f.Key] = FlagRow{Key: f.Key, Enabled: f.Default}
	}

	for _, row := range h.checker.AllFlagRows(r.Context()) {
		merged[row.Key] = row
	}

	details := make([]FeatureFlagDetail, 0, len(merged))

	for _, row := range merged {
		res := evalScoped(effectiveEnabled(row.Key, row.Enabled, row.Config), row.Config, sc)

		var cfg FlagConfig
		if len(row.Config) > 0 {
			if err := json.Unmarshal(row.Config, &cfg); err != nil {
				cfg = FlagConfig{}
			}
		}

		details = append(details, FeatureFlagDetail{
			Key:               row.Key,
			Enabled:           res.Enabled,
			Phase:             phaseFor(row.Enabled, cfg),
			RolloutPercentage: rolloutFor(cfg),
			Visible:           res.Visible,
		})
	}

	sort.Slice(details, func(i, j int) bool { return details[i].Key < details[j].Key })

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": details})
}

// HandleGetFeatureFlagDetail handles GET /api/v1/feature-flags/details/{key}.
// The single-flag form of the details list — same public/no-auth posture,
// same FeatureFlagDetail projection, same caller-scoped evaluation. Used by
// pages that only need one flag (e.g. the roadmap item's rollout badge) so
// they don't pull the entire array to filter client-side. A key that is
// neither a registry entry nor a stored feature_flags row 404s — returning
// the registry-default projection for arbitrary keys would 200 typos.
func (h *Handlers) HandleGetFeatureFlagDetail(w http.ResponseWriter, r *http.Request) {

	key := chi.URLParam(r, "key")
	if key == "" {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Feature flag not found")
		return
	}

	// Same merge shape as HandleGetFeatureFlagDetails: seed the registry
	// default, then overlay the stored row — read through the shared
	// jc:flags:_all aggregate so the lookup stays off Postgres.
	row, found := FlagRow{}, false

	if f, ok := RegistryLookup(key); ok {
		row = FlagRow{Key: f.Key, Enabled: f.Default}
		found = true
	}

	for _, stored := range h.checker.AllFlagRows(r.Context()) {
		if stored.Key == key {
			row = stored
			found = true

			break
		}
	}

	if !found {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Feature flag not found")
		return
	}

	sc := h.checker.BuildScope(r, SurfaceFrom(r))
	res := evalScoped(effectiveEnabled(row.Key, row.Enabled, row.Config), row.Config, sc)

	var cfg FlagConfig
	if len(row.Config) > 0 {
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			cfg = FlagConfig{}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": FeatureFlagDetail{
		Key:               row.Key,
		Enabled:           res.Enabled,
		Phase:             phaseFor(row.Enabled, cfg),
		RolloutPercentage: rolloutFor(cfg),
		Visible:           res.Visible,
	}})
}

// phaseFor derives the public rollout phase: the config's explicit phase
// when set, else "shipped" for enabled flags and "planned" for disabled
// ones — the same derivation eval applies to absent phases. It uses the
// flag's global enabled state, not the caller-scoped result: a flag
// geo-blocked or audience-gated for this caller is still "shipped", not
// "planned".
func phaseFor(enabled bool, cfg FlagConfig) string {
	if cfg.Phase != "" {
		return cfg.Phase
	}

	if enabled {
		return "shipped"
	}

	return "planned"
}

// rolloutFor returns the public rollout percentage, defaulting to 100 when
// the config is absent or stores 0 (same convention as applyFlagConfig).
func rolloutFor(cfg FlagConfig) int {
	if cfg.RolloutPercentage == 0 {
		return 100
	}

	return cfg.RolloutPercentage
}

// HandleSetFeatureFlag handles PATCH /admin/feature-flags/{flag_name}.
// Toggles a feature flag's enabled state for the current platform.
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

	// Uncached pre-write snapshot — a stale cached value would miss the
	// off→on transition and silently skip the activation hook. It doubles
	// as the audit row's "before" image.
	before := h.snapshotFlag(r.Context(), flagName)

	if err := h.checker.SetEnabled(r.Context(), flagName, input.Enabled, user.UserID); err != nil {
		slog.Error("features: set enabled failed", "flag", flagName, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update feature flag")

		return
	}

	// SetEnabled already invalidated the flag's cache entries and fired the
	// config-change hook.
	h.fireOnEnabled(r.Context(), flagName, before.Enabled, input.Enabled)
	h.auditFlagWrite(r.Context(), user.UserID, clientip.FromRequest(r), "feature_flag.update", flagName,
		before, &flagAuditSnapshot{Enabled: input.Enabled, Config: before.Config})

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"key":       flagName,
			"isEnabled": input.Enabled,
		},
	})
}

// FeatureFlag is the admin-facing representation of a platform feature flag.
// The storage layer only persists a flag_name → enabled mapping, so the
// richer fields (name, description, environment, rolloutPercentage) are
// derived with sensible defaults for the admin UI.
type FeatureFlag struct {
	ID                string                 `json:"id"`
	Key               string                 `json:"key"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	IsEnabled         bool                   `json:"isEnabled"`
	Environment       string                 `json:"environment"`
	RolloutPercentage int                    `json:"rolloutPercentage"`
	Phase             string                 `json:"phase,omitempty"`
	ShowOnRoadmap     bool                   `json:"showOnRoadmap"`
	Surfaces          map[string]SurfaceRule `json:"surfaces,omitempty"`
	CreatedAt         string                 `json:"createdAt"`
	UpdatedAt         string                 `json:"updatedAt"`
}

// ListFeatureFlags handles GET /api/v1/admin/feature-flags.
// Returns the platform's feature flags as an array (the public GET /feature-flags
// returns a map). Gated by admin.feature_flags.manage at the router so the admin
// SPA can render a proper list/table. It merges the static Registry keys with
// any platform rows (including admin-created keys) so the list is always complete.
func (h *Handlers) ListFeatureFlags(w http.ResponseWriter, r *http.Request) {

	rows, err := h.checker.ListFlagRows(r.Context())
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

	flag.Phase = cfg.Phase
	flag.ShowOnRoadmap = cfg.ShowOnRoadmap

	if cfg.Surfaces != nil {
		flag.Surfaces = cfg.Surfaces
	}
}

// CreateFeatureFlag handles POST /api/v1/admin/feature-flags.
// Creates a platform feature flag (registry key or new admin key) with an initial
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
		Phase:             strings.TrimSpace(input.Phase),
		ShowOnRoadmap:     input.ShowOnRoadmap,
		Surfaces:          input.Surfaces,
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

	// Phase/surface/audience/geo vocabularies are validated before the write;
	// ConfigError carries the machine-readable code (invalid_phase,
	// invalid_surface, …) emitted in the 400 error envelope.
	if err := ValidateConfig(cfg); err != nil {
		WriteConfigError(w, err)
		return
	}

	before := h.snapshotFlag(r.Context(), key)

	if err := h.checker.UpdateFlag(r.Context(), key, input.IsEnabled, cfg, user.UserID); err != nil {
		slog.Error("features: create failed", "key", key, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create feature flag")

		return
	}

	h.fireOnEnabled(r.Context(), key, before.Enabled, input.IsEnabled)
	h.auditFlagWrite(r.Context(), user.UserID, clientip.FromRequest(r), "feature_flag.create", key,
		before, &flagAuditSnapshot{Enabled: input.IsEnabled, Config: cfg})
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": FeatureFlag{
			ID:                key,
			Key:               key,
			Name:              cfg.Name,
			Description:       cfg.Description,
			IsEnabled:         input.IsEnabled,
			Environment:       cfg.Environment,
			RolloutPercentage: cfg.RolloutPercentage,
			Phase:             cfg.Phase,
			ShowOnRoadmap:     cfg.ShowOnRoadmap,
			Surfaces:          cfg.Surfaces,
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
		Phase:             strings.TrimSpace(input.Phase),
		ShowOnRoadmap:     input.ShowOnRoadmap,
		Surfaces:          input.Surfaces,
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

	// Phase/surface/audience/geo vocabularies are validated before the write;
	// ConfigError carries the machine-readable code (invalid_phase,
	// invalid_surface, …) emitted in the 400 error envelope.
	if err := ValidateConfig(cfg); err != nil {
		WriteConfigError(w, err)
		return
	}

	before := h.snapshotFlag(r.Context(), flagName)

	if err := h.checker.UpdateFlag(r.Context(), flagName, input.IsEnabled, cfg, user.UserID); err != nil {
		slog.Error("features: update failed", "key", flagName, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update feature flag")

		return
	}

	h.fireOnEnabled(r.Context(), flagName, before.Enabled, input.IsEnabled)
	h.auditFlagWrite(r.Context(), user.UserID, clientip.FromRequest(r), "feature_flag.update", flagName,
		before, &flagAuditSnapshot{Enabled: input.IsEnabled, Config: cfg})
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": FeatureFlag{
			ID:                flagName,
			Key:               flagName,
			Name:              cfg.Name,
			Description:       cfg.Description,
			IsEnabled:         input.IsEnabled,
			Environment:       cfg.Environment,
			RolloutPercentage: cfg.RolloutPercentage,
			Phase:             cfg.Phase,
			ShowOnRoadmap:     cfg.ShowOnRoadmap,
			Surfaces:          cfg.Surfaces,
		},
	})
}

// DeleteFeatureFlag handles DELETE /api/v1/admin/feature-flags/{flag_name}.
// Removes the platform's override for the flag; afterwards it reverts to its
// registry Default. Gated by admin.feature_flags.manage.
func (h *Handlers) DeleteFeatureFlag(w http.ResponseWriter, r *http.Request) {

	flagName := chi.URLParam(r, "flag_name")
	if flagName == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_flag_name", "flag_name is required")
		return
	}

	var actor pgtype.UUID
	if user := auth.FromContext(r.Context()); user != nil {
		actor = user.UserID
	}

	before := h.snapshotFlag(r.Context(), flagName)

	if err := h.checker.DeleteFlag(r.Context(), flagName); err != nil {
		slog.Error("features: delete failed", "key", flagName, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete feature flag")

		return
	}

	// DeleteFlag already invalidated the cache and fired the config-change
	// hook with an empty config (roadmap unlink per spec §4.3).
	h.auditFlagWrite(r.Context(), actor, clientip.FromRequest(r), "feature_flag.delete", flagName,
		before, nil)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"key": flagName, "deleted": true},
	})
}

// FeatureFlagInput is the request body shared by Create/Update handlers.
// Phase/ShowOnRoadmap/Surfaces carry the unified-rollout config and are
// validated by ValidateConfig before the write (invalid → 400).
type FeatureFlagInput struct {
	Name              string                 `json:"name"`
	Key               string                 `json:"key"`
	Description       string                 `json:"description"`
	IsEnabled         bool                   `json:"isEnabled"`
	Environment       string                 `json:"environment"`
	RolloutPercentage int                    `json:"rolloutPercentage"`
	Phase             string                 `json:"phase"`
	ShowOnRoadmap     bool                   `json:"showOnRoadmap"`
	Surfaces          map[string]SurfaceRule `json:"surfaces"`
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
