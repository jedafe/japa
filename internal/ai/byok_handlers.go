// Package ai — byok_handlers.go implements the user-facing BYOK surface:
// the provider allowlist endpoint that feeds the "Your keys" picker group
// and add-key dialog, the affiliate referral-click tracker, and the
// /me/ai-keys credential CRUD. Plaintext keys never leave the backend —
// every response carries api_key_masked only.
//
// Ref: docs/superpowers/specs/2026-10-19-ai-byok-design.md
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Narrow seams so handlers are testable with fakes; the concrete stores
// satisfy them.
type byokProviderLister interface {
	ListEnabled(ctx context.Context) ([]ByokProvider, error)
	Get(ctx context.Context, id pgtype.UUID) (ByokProvider, error)
	IncrementReferralClicks(ctx context.Context, id pgtype.UUID) error
}

type userAIKeyStore interface {
	ListForUser(ctx context.Context, userID pgtype.UUID) ([]UserAIKey, error)
	GetForUser(ctx context.Context, userID, keyID pgtype.UUID) (UserAIKey, error)
	Create(ctx context.Context, userID, providerID pgtype.UUID, apiKey, label string) (UserAIKey, error)
	Update(ctx context.Context, userID, keyID pgtype.UUID, label, apiKey *string) (UserAIKey, error)
	Delete(ctx context.Context, userID, keyID pgtype.UUID) error
	MarkStatus(ctx context.Context, keyID pgtype.UUID, status, lastError string) error
	ResolveProvider(ctx context.Context, userID, keyID pgtype.UUID, model string) (Provider, error)
}

// SetByokStores injects the BYOK stores. Nil-safe: handlers degrade to
// empty lists / 503 when unset.
func (h *Handlers) SetByokStores(bp byokProviderLister, uk userAIKeyStore) {
	h.byokProviders = bp
	h.userKeys = uk
}

// byokErrorStatus maps BYOK store sentinels to HTTP responses.
func byokErrorStatus(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAIKeyNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "ai_key_not_found", "API key not found")
	case errors.Is(err, ErrByokProviderNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "byok_provider_not_found", "BYOK provider not found")
	case errors.Is(err, ErrByokProviderDisabled):
		errpkg.WriteError(w, http.StatusForbidden, "byok_provider_disabled", "This provider is not accepting keys right now")
	case errors.Is(err, ErrByokModelNotAllowed):
		errpkg.WriteError(w, http.StatusBadRequest, "model_not_allowed", "Model is not in the provider's catalog")
	case errors.Is(err, ErrByokKeyInUse):
		errpkg.WriteError(w, http.StatusConflict, "key_in_use", "This API key is already attached to another account")
	default:
		slog.Error("ai: byok handler error", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Request failed")
	}
}

// byokProviderPublic is the user-facing projection of a allowlist row —
// referral_clicks, enabled, and timestamps are admin internals and stay
// off the public contract.
type byokProviderPublic struct {
	ID           pgtype.UUID `json:"id"`
	ProviderType string      `json:"provider_type"`
	DisplayName  string      `json:"display_name"`
	Models       []ModelInfo `json:"models"`
	SignupURL    string      `json:"signup_url"`
}

func byokPublic(bp ByokProvider) byokProviderPublic {
	models := bp.Models
	if models == nil {
		models = []ModelInfo{}
	}

	return byokProviderPublic{
		ID: bp.ID, ProviderType: bp.ProviderType, DisplayName: bp.DisplayName,
		Models: models, SignupURL: bp.SignupURL,
	}
}

// byokProviderView is the GET /ai/byok/providers row — the allowlist entry
// plus the caller's attached keys so one fetch feeds both the picker group
// and the add-key dialog.
type byokProviderView struct {
	byokProviderPublic
	HasKey bool             `json:"has_key"`
	Keys   []userAIKeyBrief `json:"keys"`
}

type userAIKeyBrief struct {
	ID           pgtype.UUID `json:"id"`
	Label        string      `json:"label"`
	APIKeyMasked string      `json:"api_key_masked"`
	Status       string      `json:"status"`
}

// userAIKeyPublic is the /me/ai-keys response shape — same fields as
// UserAIKey but the embedded provider goes through byokProviderPublic so
// admin internals (referral_clicks, enabled, timestamps) stay off the wire.
type userAIKeyPublic struct {
	ID             pgtype.UUID        `json:"id"`
	ByokProviderID pgtype.UUID        `json:"byok_provider_id"`
	Label          string             `json:"label"`
	APIKeyMasked   string             `json:"api_key_masked"`
	Status         string             `json:"status"`
	LastError      string             `json:"last_error,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Provider       byokProviderPublic `json:"provider"`
}

func byokKeyPublic(k UserAIKey) userAIKeyPublic {
	return userAIKeyPublic{
		ID: k.ID, ByokProviderID: k.ByokProviderID, Label: k.Label,
		APIKeyMasked: k.APIKeyMasked, Status: k.Status, LastError: k.LastError,
		CreatedAt: k.CreatedAt, UpdatedAt: k.UpdatedAt, Provider: byokPublic(k.Provider),
	}
}

// HandleListByokProviders serves GET /ai/byok/providers.
func (h *Handlers) HandleListByokProviders(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.byokProviders == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	providers, err := h.byokProviders.ListEnabled(r.Context())
	if err != nil {
		slog.Error("ai: list byok providers", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list providers")

		return
	}

	var keys []UserAIKey
	if h.userKeys != nil {
		if ks, kerr := h.userKeys.ListForUser(r.Context(), user.UserID); kerr == nil {
			keys = ks
		} else {
			slog.Warn("ai: list user ai keys for byok view", "error", kerr)
		}
	}

	keysByProvider := make(map[pgtype.UUID][]userAIKeyBrief, len(keys))
	for _, k := range keys {
		keysByProvider[k.ByokProviderID] = append(keysByProvider[k.ByokProviderID], userAIKeyBrief{
			ID:           k.ID,
			Label:        k.Label,
			APIKeyMasked: k.APIKeyMasked,
			Status:       k.Status,
		})
	}

	out := make([]byokProviderView, 0, len(providers))
	for _, bp := range providers {
		ks := keysByProvider[bp.ID]
		if ks == nil {
			ks = []userAIKeyBrief{}
		}

		out = append(out, byokProviderView{
			byokProviderPublic: byokPublic(bp),
			HasKey:             len(ks) > 0,
			Keys:               ks,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// HandleByokReferralClick serves POST /ai/byok/providers/{id}/referral-click.
// Fire-and-forget counter for affiliate analytics.
func (h *Handlers) HandleByokReferralClick(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	if h.byokProviders == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_store", "BYOK not configured")
		return
	}

	if _, err := h.byokProviders.Get(r.Context(), id); err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "byok_provider_not_found", "BYOK provider not found")
		return
	}

	// One click per user per provider per day — raw spam would inflate the
	// affiliate counter and distort reporting.
	already := false
	if h.cache != nil {
		ck := fmt.Sprintf("jc:ai:byok:refclick:%s:%s", user.UserID.String(), id.String())

		if v, gerr := h.cache.Get(r.Context(), ck); gerr == nil && v != "" {
			already = true
		} else {
			_ = h.cache.Set(r.Context(), ck, "1", 86400)
		}
	}

	if !already {
		if err := h.byokProviders.IncrementReferralClicks(r.Context(), id); err != nil {
			slog.Warn("ai: referral click increment failed", "provider_id", id, "error", err)
		}
	}

	slog.Info("ai: byok referral click", "provider_id", id, "user_id", user.UserID)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": true}})
}

// HandleListMyAIKeys serves GET /me/ai-keys.
func (h *Handlers) HandleListMyAIKeys(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.userKeys == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	keys, err := h.userKeys.ListForUser(r.Context(), user.UserID)
	if err != nil {
		slog.Error("ai: list user ai keys", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list keys")

		return
	}

	out := make([]userAIKeyPublic, 0, len(keys))
	for _, k := range keys {
		out = append(out, byokKeyPublic(k))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// HandleCreateMyAIKey serves POST /me/ai-keys.
func (h *Handlers) HandleCreateMyAIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		ByokProviderID string `json:"byok_provider_id"`
		APIKey         string `json:"api_key"`
		Label          string `json:"label"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	var providerID pgtype.UUID
	if err := providerID.Scan(input.ByokProviderID); err != nil || !providerID.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "byok_provider_id must be a UUID")
		return
	}

	apiKey := strings.TrimSpace(input.APIKey)
	label := strings.TrimSpace(input.Label)

	if apiKey == "" || len(apiKey) > 4096 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "api_key is required (max 4096 chars)")
		return
	}

	if len(label) > 120 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "label is too long (max 120 chars)")
		return
	}

	if h.userKeys == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_store", "BYOK not configured")
		return
	}

	// Abuse control: cap keys per user (key farming — registering hundreds
	// of free-tier/stolen credentials to launder provider rate limits).
	maxKeys := 10
	if h.Settings != nil {
		maxKeys = h.Settings.GetIntOr(r.Context(), "ai.byok.max_keys_per_user", 10)
	}

	if existing, lerr := h.userKeys.ListForUser(r.Context(), user.UserID); lerr == nil && len(existing) >= maxKeys {
		errpkg.WriteError(w, http.StatusForbidden, "key_limit_reached", "Maximum number of API keys reached — remove one first")
		return
	}

	// Reject throttle: repeated 401s look like key-probing/laundering
	// (trying lists of credentials until one sticks). After
	// ai.byok.max_reject_attempts within the hour the create endpoint is
	// temporarily blocked for this user.
	if h.cache != nil {
		rejectKey := fmt.Sprintf("jc:ai:byok:rejects:%s", user.UserID.String())

		if raw, gerr := h.cache.Get(r.Context(), rejectKey); gerr == nil && raw != "" {
			maxRejects := 5
			if h.Settings != nil {
				maxRejects = h.Settings.GetIntOr(r.Context(), "ai.byok.max_reject_attempts", 5)
			}

			if n, perr := strconv.Atoi(raw); perr == nil && n >= maxRejects {
				errpkg.WriteError(w, http.StatusTooManyRequests, "too_many_rejected_keys", "Too many rejected keys — try again later")
				return
			}
		}
	}

	// Verify-before-save: dead credentials are rejected instead of stored.
	// Transient provider failures don't block the save — the key lands
	// unverified and the user can re-verify later.
	if h.byokKeyRejected(r.Context(), providerID, apiKey) {
		h.bumpByokRejects(r.Context(), user.UserID)
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "key_rejected", "The provider rejected this API key")
		return
	}

	key, err := h.userKeys.Create(r.Context(), user.UserID, providerID, apiKey, label)
	if err != nil {
		byokErrorStatus(w, err)
		return
	}

	// Successful save clears the probe counter — a legitimate user who
	// mistyped a few keys isn't throttled for the rest of the hour.
	if h.cache != nil {
		_ = h.cache.Set(r.Context(), fmt.Sprintf("jc:ai:byok:rejects:%s", user.UserID.String()), "0", 3600)
	}

	slog.Info("ai: user api key created", "user_id", user.UserID, "provider_id", providerID, "key_id", key.ID)
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": byokKeyPublic(key)})
}

// HandleUpdateMyAIKey serves PATCH /me/ai-keys/{id}.
func (h *Handlers) HandleUpdateMyAIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var keyID pgtype.UUID
	if err := keyID.Scan(chi.URLParam(r, "id")); err != nil || !keyID.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	var input struct {
		Label  *string `json:"label"`
		APIKey *string `json:"api_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.Label != nil {
		l := strings.TrimSpace(*input.Label)
		input.Label = &l
	}
	if input.APIKey != nil {
		k := strings.TrimSpace(*input.APIKey)
		input.APIKey = &k
	}

	if input.Label == nil && (input.APIKey == nil || *input.APIKey == "") {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "nothing to update")
		return
	}

	if input.Label != nil && len(*input.Label) > 120 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "label is too long (max 120 chars)")
		return
	}

	if input.APIKey != nil && len(*input.APIKey) > 4096 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "api_key is too long")
		return
	}

	if h.userKeys == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_store", "BYOK not configured")
		return
	}

	// Rotating to a dead credential would silently flag 'active' again —
	// re-verify before persisting, same as create. The probe throttle is
	// shared so probing via rotate is covered too.
	if input.APIKey != nil && *input.APIKey != "" {
		existing, gerr := h.userKeys.GetForUser(r.Context(), user.UserID, keyID)
		if gerr != nil {
			byokErrorStatus(w, gerr)
			return
		}

		if h.byokKeyRejected(r.Context(), existing.ByokProviderID, *input.APIKey) {
			h.bumpByokRejects(r.Context(), user.UserID)
			errpkg.WriteError(w, http.StatusUnprocessableEntity, "key_rejected", "The provider rejected this API key")
			return
		}
	}

	key, err := h.userKeys.Update(r.Context(), user.UserID, keyID, input.Label, input.APIKey)
	if err != nil {
		byokErrorStatus(w, err)
		return
	}

	slog.Info("ai: user api key updated", "user_id", user.UserID, "key_id", keyID)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": byokKeyPublic(key)})
}

// HandleDeleteMyAIKey serves DELETE /me/ai-keys/{id}.
func (h *Handlers) HandleDeleteMyAIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var keyID pgtype.UUID
	if err := keyID.Scan(chi.URLParam(r, "id")); err != nil || !keyID.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	if h.userKeys == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_store", "BYOK not configured")
		return
	}

	if err := h.userKeys.Delete(r.Context(), user.UserID, keyID); err != nil {
		byokErrorStatus(w, err)
		return
	}

	slog.Info("ai: user api key deleted", "user_id", user.UserID, "key_id", keyID)
	w.WriteHeader(http.StatusNoContent)
}

// byokKeyRejected builds an ephemeral provider from the submitted
// credential and calls FetchModels; true only when the provider rejects
// the credential outright (401). Transient failures return false — they
// must not block saves or mark keys dead.
func (h *Handlers) byokKeyRejected(ctx context.Context, providerID pgtype.UUID, apiKey string) bool {
	if h.byokProviders == nil {
		return false
	}

	bp, err := h.byokProviders.Get(ctx, providerID)
	if err != nil || !bp.Enabled {
		return false
	}

	p, perr := NewProviderFromConfig(ProviderConfig{
		Name:         bp.DisplayName,
		ProviderType: bp.ProviderType,
	}, apiKey)
	if perr != nil {
		return false
	}

	vctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, verr := p.FetchModels(vctx)

	return IsProviderKeyRejected(verr)
}

// bumpByokRejects increments the probe-throttle counter for a user.
func (h *Handlers) bumpByokRejects(ctx context.Context, userID pgtype.UUID) {
	if h.cache == nil {
		return
	}

	rk := fmt.Sprintf("jc:ai:byok:rejects:%s", userID.String())
	n := 1

	if raw, gerr := h.cache.Get(ctx, rk); gerr == nil && raw != "" {
		if v, perr := strconv.Atoi(raw); perr == nil {
			n = v + 1
		}
	}

	_ = h.cache.Set(ctx, rk, strconv.Itoa(n), 3600)
}

// HandleVerifyMyAIKey serves POST /me/ai-keys/{id}/verify. Uses the
// provider's FetchModels endpoint as the cheapest possible liveness check;
// a 401 marks the key invalid, anything else leaves status untouched.
func (h *Handlers) HandleVerifyMyAIKey(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var keyID pgtype.UUID
	if err := keyID.Scan(chi.URLParam(r, "id")); err != nil || !keyID.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID")
		return
	}

	if h.userKeys == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "no_store", "BYOK not configured")
		return
	}

	if _, err := h.userKeys.GetForUser(r.Context(), user.UserID, keyID); err != nil {
		byokErrorStatus(w, err)
		return
	}

	p, err := h.userKeys.ResolveProvider(r.Context(), user.UserID, keyID, "")
	if err != nil {
		byokErrorStatus(w, err)
		return
	}

	vctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	_, verr := p.FetchModels(vctx)

	switch {
	case verr == nil:
		_ = h.userKeys.MarkStatus(r.Context(), keyID, "active", "")
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": "active"}})
	case IsProviderKeyRejected(verr):
		_ = h.userKeys.MarkStatus(r.Context(), keyID, "invalid", verr.Error())
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": "invalid"}})
	default:
		// Transient failure (network, 5xx, rate limit) — don't flag the key.
		slog.Warn("ai: key verification unreachable", "key_id", keyID, "error", verr)
		errpkg.WriteError(w, http.StatusBadGateway, "verification_failed", "Could not reach the provider — try again later")
	}
}
