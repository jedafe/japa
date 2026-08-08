package spam

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

const (
	defaultPageSize int32 = 50
	maxPageSize     int32 = 200
)

// deleteByID is a shared helper for DELETE handlers that follow the pattern:
// require user → require tenant → parse UUID → call delete function → return.
func deleteByID(w http.ResponseWriter, r *http.Request, entityName string, deleteFn func(ctx context.Context, id, tenantID pgtype.UUID) error) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid "+entityName+" ID")
		return
	}

	if err := deleteFn(r.Context(), id, tenantID); err != nil {
		slog.Error("deleting "+entityName, "error", err)
		handleServiceError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// Handlers holds dependencies for spam admin HTTP handlers.
type Handlers struct {
	Store *Store
}

// NewHandlers creates Handlers with the given Store.
func NewHandlers(store *Store) *Handlers {
	return &Handlers{Store: store}
}

// --- helpers ---

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

func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > int(maxPageSize) {
		return maxPageSize
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

func parseCursorCreatedAt(r *http.Request) pgtype.Timestamptz {
	s := r.URL.Query().Get("cursor")
	if s == "" {
		return pgtype.Timestamptz{}
	}

	var ts pgtype.Timestamptz
	if err := ts.Scan(s); err != nil {
		return pgtype.Timestamptz{}
	}

	return ts
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

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// ---------------------------------------------------------------------------
// IP Block Handlers
// ---------------------------------------------------------------------------

// ListIPBlocks handles GET /admin/spam/ip-blocks.
func (h *Handlers) ListIPBlocks(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	cursor := parseCursorCreatedAt(r)

	blocks, err := h.Store.ListIPBlocks(r.Context(), tenantID, limit, cursor)
	if err != nil {
		slog.Error("listing ip blocks", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list IP blocks")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": blocks})
}

// CreateIPBlock handles POST /admin/spam/ip-blocks.
func (h *Handlers) CreateIPBlock(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		CIDR      string  `json:"cidr"`
		BlockType string  `json:"block_type"`
		Reason    *string `json:"reason"`
		ExpiresAt string  `json:"expires_at"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.CIDR == "" {
		writeError(w, http.StatusBadRequest, "missing_cidr", "cidr is required")
		return
	}

	if input.BlockType == "" {
		writeError(w, http.StatusBadRequest, "missing_block_type", "block_type is required")
		return
	}

	if !IsValidBlockType(input.BlockType) {
		writeError(w, http.StatusBadRequest, "invalid_block_type", "block_type must be one of: full, registration, posting, api")
		return
	}

	var expiresAt pgtype.Timestamptz
	if input.ExpiresAt != "" {
		if err := expiresAt.Scan(input.ExpiresAt); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_expires_at", "expires_at must be a valid timestamp")
			return
		}
	}

	block, err := h.Store.CreateIPBlock(r.Context(), CreateIPBlockInput{
		TenantID:  tenantID,
		CIDR:      input.CIDR,
		BlockType: input.BlockType,
		Reason:    input.Reason,
		BlockedBy: user.UserID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		slog.Error("creating ip block", "error", err)
		handleServiceError(w, err)

		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": block})
}

// UpdateIPBlock handles PATCH /admin/spam/ip-blocks/{id}.
func (h *Handlers) UpdateIPBlock(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid IP block ID")
		return
	}

	var input struct {
		CIDR      *string `json:"cidr"`
		BlockType *string `json:"block_type"`
		Reason    *string `json:"reason"`
		ExpiresAt *string `json:"expires_at"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.BlockType != nil && !IsValidBlockType(*input.BlockType) {
		writeError(w, http.StatusBadRequest, "invalid_block_type", "block_type must be one of: full, registration, posting, api")
		return
	}

	var expiresAt *pgtype.Timestamptz

	if input.ExpiresAt != nil && *input.ExpiresAt != "" {
		var ts pgtype.Timestamptz
		if err := ts.Scan(*input.ExpiresAt); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_expires_at", "expires_at must be a valid timestamp")
			return
		}

		expiresAt = &ts
	}

	block, err := h.Store.UpdateIPBlock(r.Context(), id, tenantID, UpdateIPBlockInput{
		CIDR:      input.CIDR,
		BlockType: input.BlockType,
		Reason:    input.Reason,
		ExpiresAt: expiresAt,
		BlockedBy: user.UserID,
	})
	if err != nil {
		slog.Error("updating ip block", "error", err)
		handleServiceError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": block})
}

// DeleteIPBlock handles DELETE /admin/spam/ip-blocks/{id}.
func (h *Handlers) DeleteIPBlock(w http.ResponseWriter, r *http.Request) {
	deleteByID(w, r, "IP block", func(ctx context.Context, id, tenantID pgtype.UUID) error {
		return h.Store.DeleteIPBlock(ctx, id, tenantID)
	})
}

// ---------------------------------------------------------------------------
// Censored Word Handlers
// ---------------------------------------------------------------------------

// ListCensoredWords handles GET /admin/spam/censored-words.
func (h *Handlers) ListCensoredWords(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	words, err := h.Store.ListCensoredWords(r.Context(), tenantID)
	if err != nil {
		slog.Error("listing censored words", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list censored words")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": words})
}

// CreateCensoredWord handles POST /admin/spam/censored-words.
func (h *Handlers) CreateCensoredWord(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Pattern     string  `json:"pattern"`
		Action      string  `json:"action"`
		Replacement *string `json:"replacement"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Pattern == "" {
		writeError(w, http.StatusBadRequest, "missing_pattern", "pattern is required")
		return
	}

	if input.Action == "" {
		writeError(w, http.StatusBadRequest, "missing_action", "action is required")
		return
	}

	if !IsValidCensorAction(input.Action) {
		writeError(w, http.StatusBadRequest, "invalid_action", "action must be one of: block, replace, flag")
		return
	}

	if input.Action == CensorActionReplace && (input.Replacement == nil || *input.Replacement == "") {
		writeError(w, http.StatusBadRequest, "missing_replacement", "replacement is required when action is replace")
		return
	}

	word, err := h.Store.CreateCensoredWord(r.Context(), CreateCensoredWordInput{
		TenantID:    tenantID,
		Pattern:     input.Pattern,
		Action:      input.Action,
		Replacement: input.Replacement,
	})
	if err != nil {
		slog.Error("creating censored word", "error", err)
		handleServiceError(w, err)

		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": word})
}

// UpdateCensoredWord handles PATCH /admin/spam/censored-words/{id}.
func (h *Handlers) UpdateCensoredWord(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid censored word ID")
		return
	}

	var input struct {
		Pattern     *string `json:"pattern"`
		Action      *string `json:"action"`
		Replacement *string `json:"replacement"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Action != nil && !IsValidCensorAction(*input.Action) {
		writeError(w, http.StatusBadRequest, "invalid_action", "action must be one of: block, replace, flag")
		return
	}

	if input.Action != nil && *input.Action == CensorActionReplace && (input.Replacement == nil || *input.Replacement == "") {
		writeError(w, http.StatusBadRequest, "missing_replacement", "replacement is required when action is replace")
		return
	}

	word, err := h.Store.UpdateCensoredWord(r.Context(), id, tenantID, UpdateCensoredWordInput{
		Pattern:     input.Pattern,
		Action:      input.Action,
		Replacement: input.Replacement,
	})
	if err != nil {
		slog.Error("updating censored word", "error", err)
		handleServiceError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": word})
}

// DeleteCensoredWord handles DELETE /admin/spam/censored-words/{id}.
func (h *Handlers) DeleteCensoredWord(w http.ResponseWriter, r *http.Request) {
	deleteByID(w, r, "censored word", func(ctx context.Context, id, tenantID pgtype.UUID) error {
		return h.Store.DeleteCensoredWord(ctx, id, tenantID)
	})
}

// ---------------------------------------------------------------------------
// User Block Handlers
// ---------------------------------------------------------------------------

// ListUserBlocks handles GET /admin/spam/user-blocks.
func (h *Handlers) ListUserBlocks(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	limit := parsePageSize(r)

	blocks, err := h.Store.ListAllUserBlocks(r.Context(), limit)
	if err != nil {
		slog.Error("listing user blocks", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list user blocks")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": blocks})
}

// DeleteUserBlock handles DELETE /admin/spam/user-blocks/{blockerId}/{blockedId}.
func (h *Handlers) DeleteUserBlock(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	blockerID, err := parseUUIDParam(r, "blockerId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_blocker_id", "Invalid blocker ID")
		return
	}

	blockedID, err := parseUUIDParam(r, "blockedId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_blocked_id", "Invalid blocked ID")
		return
	}

	if err := h.Store.DeleteUserBlock(r.Context(), tenantID, blockerID, blockedID); err != nil {
		slog.Error("deleting user block", "error", err)
		handleServiceError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}
