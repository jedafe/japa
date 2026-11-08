package nextactions

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the §122 public + admin endpoints.
type Handlers struct {
	svc      *Service
	settings *config.Settings
	auditDB  gen.DBTX
}

// NewHandlers wires handlers to the engine.
func NewHandlers(svc *Service, settings *config.Settings) *Handlers {
	return &Handlers{svc: svc, settings: settings}
}

// SetAuditDB wires the writer pool used for audit_logs inserts. Optional —
// without it rule mutations simply skip the audit row (tests).
func (h *Handlers) SetAuditDB(db gen.DBTX) { h.auditDB = db }

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
		if errors.Is(err, ErrRuleNotServed) {
			errpkg.WriteError(w, http.StatusNotFound, "rule_not_served", "This suggestion was not served to you")
			return
		}
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
		if errors.Is(err, ErrRuleNotServed) {
			errpkg.WriteError(w, http.StatusNotFound, "rule_not_served", "This suggestion was not served to you")
			return
		}
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
		if errors.Is(err, ErrRuleNotServed) {
			errpkg.WriteError(w, http.StatusNotFound, "rule_not_served", "This suggestion was not served to you")
			return
		}
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

const (
	defaultRulePageSize int32 = 20
	maxRulePageSize     int32 = 100
)

// parseRulePageSize extracts and clamps ?page_size= (api-response-format.md).
func parseRulePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultRulePageSize
	}
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil || v <= 0 || v > int64(maxRulePageSize) {
		return defaultRulePageSize
	}
	return int32(v) // #nosec G115 // bounded above
}

// parseRulePageOffset extracts ?page_offset=, or 1-based ?page= → offset.
func parseRulePageOffset(r *http.Request, limit int32) int32 {
	if s := r.URL.Query().Get("page_offset"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 32); err == nil && v >= 0 {
			return int32(v) // #nosec G115 // bounded above by clamp below
		}
		return 0
	}
	if s := r.URL.Query().Get("page"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 32); err == nil && v >= 1 && v <= 1000000 {
			return int32(v-1) * limit // #nosec G115 -- page<=1e6
		}
	}
	return 0
}

// ListRules handles GET /api/v1/admin/next-actions/rules — offset-paginated
// per api-response-format.md: ?q (slug/name ilike), ?enabled, ?sort +
// ?sort_dir (whitelisted column keys), ?page_size/&page_offset (or 1-based
// ?page). Response rides the standard {data, total, page, total_pages}
// envelope the control DataTable consumes server-side.
func (h *Handlers) ListRules(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	enabled := r.URL.Query().Get("enabled")
	if enabled != "true" && enabled != "false" {
		enabled = ""
	}
	sortParam := r.URL.Query().Get("sort")
	sortDir := r.URL.Query().Get("sort_dir")
	limit := parseRulePageSize(r)
	offset := parseRulePageOffset(r, limit)

	rows, err := h.svc.read.ListNextActionRules(r.Context(), gen.ListNextActionRulesParams{
		Q: q, Enabled: enabled,
		Sort: sortParam, SortDir: sortDir,
		PageOffset: offset, PageLimit: limit,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "rules_failed", "Failed to list rules")
		return
	}
	count, err := h.svc.read.CountNextActionRules(r.Context(), gen.CountNextActionRulesParams{
		Q: q, Enabled: enabled,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "rules_failed", "Failed to count rules")
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

	totalPages := int32(1)
	if limit > 0 {
		totalPages = (int32(count) + limit - 1) / limit // #nosec G115 // bounded by caller
		if totalPages < 1 {
			totalPages = 1
		}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"data":        out,
			"total":       count,
			"page":        offset / limit,
			"per_page":    limit,
			"total_pages": totalPages,
		},
	})
}

// GetRule handles GET /api/v1/admin/next-actions/rules/{id}.
func (h *Handlers) GetRule(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ruleID(w, r)
	if !ok {
		return
	}
	row, err := h.svc.read.GetNextActionRule(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "rule_not_found", "Rule not found")
			return
		}
		errpkg.WriteError(w, http.StatusInternalServerError, "rule_lookup_failed", "Failed to load rule")
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
	h.audit(r, "nextactions.rule.create", rule.Slug, row.ID)
	out, err := decodeRule(row)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "rule_decode_failed", "Rule saved but failed to decode")
		return
	}
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
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "rule_not_found", "Rule not found")
			return
		}
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update rule")
		return
	}
	h.svc.InvalidateRulesCache(r.Context())
	h.audit(r, "nextactions.rule.update", rule.Slug, row.ID)
	out, err := decodeRule(row)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "rule_decode_failed", "Rule saved but failed to decode")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// DeleteRule handles DELETE /api/v1/admin/next-actions/rules/{id}.
func (h *Handlers) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ruleID(w, r)
	if !ok {
		return
	}
	// Grab the slug for the audit row before the rule is gone — best-effort:
	// a failed lookup must not block the delete.
	var slug string
	if existing, err := h.svc.read.GetNextActionRule(r.Context(), id); err == nil {
		slug = existing.Slug
	}
	if err := h.svc.write.DeleteNextActionRule(r.Context(), id); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete rule")
		return
	}
	h.svc.InvalidateRulesCache(r.Context())
	h.audit(r, "nextactions.rule.delete", slug, id)
	w.WriteHeader(http.StatusNoContent)
}

// Predicates handles GET /api/v1/admin/next-actions/predicates — the
// code-registered predicate vocabulary + arg schema for the Control
// structured rule builder.
func (h *Handlers) Predicates(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": PredicateSpecs()})
}

// Vocab handles GET /api/v1/admin/next-actions/vocab — the bounded
// vocabularies (channels, user classes, surfaces) the Control rule builder
// renders, so the frontend shares the backend enums instead of hardcoding
// them (Rule 16 — option lists must be API-driven).
func (h *Handlers) Vocab(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"channels":     VocabChannels(),
		"user_classes": VocabUserClasses(),
		"surfaces":     VocabSurfaces(),
	}})
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

// audit writes a best-effort audit_logs row for a rule mutation — mirroring
// the admin/settings.go convention: who changed what and when must be
// traceable, but a failed insert never blocks the mutation itself.
func (h *Handlers) audit(r *http.Request, actionType, slug string, id pgtype.UUID) {
	if h.auditDB == nil {
		return
	}
	var actorID pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		actorID = u.UserID
	}
	meta, _ := json.Marshal(map[string]any{"slug": slug})
	ip := clientip.FromRequest(r)
	var ipArg any
	if ip != "" {
		ipArg = ip
	}
	if _, err := h.auditDB.Exec(r.Context(), `
		INSERT INTO audit_logs (actor_user_id, action_type, target_type, target_id, metadata, ip_address)
		VALUES ($1, $2, 'next_action_rule', $3, $4::jsonb, $5::inet)
	`, actorID, actionType, id, meta, ipArg); err != nil {
		slog.Warn("nextactions: failed to write rule audit log", "action", actionType, "error", err)
	}
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
