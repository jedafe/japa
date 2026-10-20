package nextactions

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the §122 public + admin endpoints.
type Handlers struct {
	svc      *Service
	settings *config.Settings
}

// NewHandlers wires handlers to the engine.
func NewHandlers(svc *Service, settings *config.Settings) *Handlers {
	return &Handlers{svc: svc, settings: settings}
}

const maxSnoozeHours = 24 * 365 // 1 year clamp

// GetSuggestions handles GET /api/v1/next-actions and GET /api/v1/me/next-actions.
func (h *Handlers) GetSuggestions(w http.ResponseWriter, r *http.Request) {
	surface := r.URL.Query().Get("surface")
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v) //nolint:errcheck // invalid → default
	}
	if limit < 0 {
		limit = 0
	}
	if limit > 50 {
		limit = 50
	}
	sugs, err := h.svc.Suggestions(r.Context(), r, surface, limit)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "next_actions_failed", "Failed to load suggestions")
		return
	}
	if sugs == nil {
		sugs = []Suggestion{}
	}
	// meta carries the render-time caps the server owns — renderers enforce
	// popup_max_per_session client-side instead of hardcoding it (§122).
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": sugs,
		"meta": map[string]any{
			"popup_max_per_session": h.settingInt(r, "nextactions.popup_max_per_session", 1),
		},
	})
}

func (h *Handlers) settingInt(r *http.Request, key string, fallback int) int {
	if h.settings == nil {
		return fallback
	}
	return h.settings.GetIntOr(r.Context(), key, fallback)
}

func (h *Handlers) ruleID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_next_action", "Invalid suggestion id")
		return id, false
	}
	return id, true
}

// Dismiss handles POST /api/v1/next-actions/{id}/dismiss {snooze_hours?}.
func (h *Handlers) Dismiss(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ruleID(w, r)
	if !ok {
		return
	}
	var body struct {
		SnoozeHours int `json:"snooze_hours"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // empty body is fine
	if body.SnoozeHours < 0 {
		body.SnoozeHours = 0
	}
	if body.SnoozeHours > maxSnoozeHours {
		body.SnoozeHours = maxSnoozeHours
	}
	if err := h.svc.Dismiss(r.Context(), r, id, body.SnoozeHours); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "dismiss_failed", "Failed to dismiss suggestion")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"dismissed": true}})
}

// Complete handles POST /api/v1/next-actions/{id}/complete.
func (h *Handlers) Complete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ruleID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Complete(r.Context(), r, id); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "complete_failed", "Failed to complete suggestion")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"completed": true}})
}

// Track handles POST /api/v1/next-actions/{id}/track {event: shown|clicked}.
func (h *Handlers) Track(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ruleID(w, r)
	if !ok {
		return
	}
	var body struct {
		Event string `json:"event"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Event != "shown" && body.Event != "clicked") {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_track_event", "event must be shown or clicked")
		return
	}
	if err := h.svc.Track(r.Context(), r, id, body.Event); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "track_failed", "Failed to record event")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Admin (Control panel) — rule CRUD + stats
// ---------------------------------------------------------------------------

// ruleRequest is the admin create/update payload — jsonb columns arrive as
// raw JSON and are validated through the typed structs.
type ruleRequest struct {
	Slug             string          `json:"slug"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	Enabled          bool            `json:"enabled"`
	Priority         int32           `json:"priority"`
	Upsell           bool            `json:"upsell"`
	Audience         Audience        `json:"audience"`
	Conditions       ConditionGroup  `json:"conditions"`
	Payload          Payload         `json:"payload"`
	Channels         []string        `json:"channels"`
	CompletionSignal *ConditionGroup `json:"completion_signal"`
	Frequency        Frequency       `json:"frequency"`
	ValidFrom        *string         `json:"valid_from"`
	ValidUntil       *string         `json:"valid_until"`
}

func (rr *ruleRequest) toRule(id pgtype.UUID) (Rule, error) {
	rule := Rule{
		ID: id, Slug: rr.Slug, Name: rr.Name, Description: rr.Description,
		Enabled: rr.Enabled, Priority: rr.Priority, Upsell: rr.Upsell, Audience: rr.Audience,
		Conditions: rr.Conditions, Payload: rr.Payload, Channels: rr.Channels,
		CompletionSignal: rr.CompletionSignal, Frequency: rr.Frequency,
	}
	for _, ts := range []struct {
		raw string
		dst *pgtype.Timestamptz
	}{
		{deref(rr.ValidFrom), &rule.ValidFrom},
		{deref(rr.ValidUntil), &rule.ValidUntil},
	} {
		if ts.raw == "" {
			continue
		}
		if err := ts.dst.Scan(ts.raw); err != nil {
			return rule, err
		}
	}
	return rule, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ListRules handles GET /api/v1/admin/next-actions/rules.
func (h *Handlers) ListRules(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.read.ListNextActionRules(r.Context())
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "rules_failed", "Failed to list rules")
		return
	}
	out := make([]Rule, 0, len(rows))
	for i := range rows {
		rule, err := decodeRule(rows[i])
		if err != nil {
			// Surface corrupt rows as stubs rather than silently dropping
			// them — an undecodable rule must stay visible for admin repair.
			slog.Warn("nextactions: list rule decode failed", "slug", rows[i].Slug, "error", err)
			out = append(out, Rule{
				ID: rows[i].ID, Slug: rows[i].Slug, Name: rows[i].Name,
				Description: rows[i].Description, Enabled: rows[i].Enabled,
				Priority: rows[i].Priority, Upsell: rows[i].Upsell,
				ValidFrom: rows[i].ValidFrom, ValidUntil: rows[i].ValidUntil,
				CreatedAt: rows[i].CreatedAt, UpdatedAt: rows[i].UpdatedAt,
			})
			continue
		}
		out = append(out, rule)
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// GetRule handles GET /api/v1/admin/next-actions/rules/{id}.
func (h *Handlers) GetRule(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ruleID(w, r)
	if !ok {
		return
	}
	row, err := h.svc.read.GetNextActionRule(r.Context(), id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "rule_not_found", "Rule not found")
		return
	}
	rule, err := decodeRule(row)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "rule_decode_failed", "Failed to decode rule")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rule})
}

// CreateRule handles POST /api/v1/admin/next-actions/rules.
func (h *Handlers) CreateRule(w http.ResponseWriter, r *http.Request) {
	var req ruleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	rule, err := req.toRule(pgtype.UUID{})
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_rule", err.Error())
		return
	}
	if err := ValidateRule(rule, h.origins(r)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_rule", err.Error())
		return
	}
	var createdBy pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		createdBy = u.UserID
	}
	row, err := h.svc.write.CreateNextActionRule(r.Context(), gen.CreateNextActionRuleParams{
		Slug: rule.Slug, Name: rule.Name, Description: rule.Description,
		Enabled: rule.Enabled, Priority: rule.Priority, Upsell: rule.Upsell,
		Audience: mustJSON(rule.Audience), Conditions: mustJSON(rule.Conditions),
		Payload: mustJSON(rule.Payload), Channels: mustJSON(rule.Channels),
		CompletionSignal: maybeJSON(rule.CompletionSignal), Frequency: mustJSON(rule.Frequency),
		ValidFrom: rule.ValidFrom, ValidUntil: rule.ValidUntil, CreatedBy: createdBy,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			errpkg.WriteError(w, http.StatusConflict, "rule_slug_conflict", "A rule with this slug already exists")
			return
		}
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create rule")
		return
	}
	h.svc.InvalidateRulesCache(r.Context())
	out, _ := decodeRule(row)
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": out})
}

// UpdateRule handles PATCH /api/v1/admin/next-actions/rules/{id}.
func (h *Handlers) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ruleID(w, r)
	if !ok {
		return
	}
	var req ruleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	rule, err := req.toRule(id)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_rule", err.Error())
		return
	}
	if strings.TrimSpace(rule.Slug) == "" {
		// Slug is immutable — UPDATE never writes it, so a PATCH omitting
		// slug borrows the persisted one for validation.
		if existing, err := h.svc.read.GetNextActionRule(r.Context(), id); err == nil {
			rule.Slug = existing.Slug
		}
	}
	if err := ValidateRule(rule, h.origins(r)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_rule", err.Error())
		return
	}
	row, err := h.svc.write.UpdateNextActionRule(r.Context(), gen.UpdateNextActionRuleParams{
		ID: id, Name: rule.Name, Description: rule.Description,
		Enabled: rule.Enabled, Priority: rule.Priority, Upsell: rule.Upsell,
		Audience: mustJSON(rule.Audience), Conditions: mustJSON(rule.Conditions),
		Payload: mustJSON(rule.Payload), Channels: mustJSON(rule.Channels),
		CompletionSignal: maybeJSON(rule.CompletionSignal), Frequency: mustJSON(rule.Frequency),
		ValidFrom: rule.ValidFrom, ValidUntil: rule.ValidUntil,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update rule")
		return
	}
	h.svc.InvalidateRulesCache(r.Context())
	out, _ := decodeRule(row)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// DeleteRule handles DELETE /api/v1/admin/next-actions/rules/{id}.
func (h *Handlers) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ruleID(w, r)
	if !ok {
		return
	}
	if err := h.svc.write.DeleteNextActionRule(r.Context(), id); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete rule")
		return
	}
	h.svc.InvalidateRulesCache(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// Predicates handles GET /api/v1/admin/next-actions/predicates — the
// code-registered predicate vocabulary + arg schema for the Control
// structured rule builder.
func (h *Handlers) Predicates(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": PredicateSpecs()})
}

// Stats handles GET /api/v1/admin/next-actions/stats.
func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.read.NextActionStats(r.Context())
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "stats_failed", "Failed to load stats")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// origins decodes the nextactions.allowed_cta_origins setting for validation.
func (h *Handlers) origins(r *http.Request) []string {
	if h.settings == nil {
		return nil
	}
	raw := h.settings.GetStringOr(r.Context(), "nextactions.allowed_cta_origins", "")
	var out []string
	for _, o := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(o); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func maybeJSON(v *ConditionGroup) []byte {
	if v == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return b
}
