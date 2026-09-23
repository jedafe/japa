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
	"github.com/jedafe/japa/internal/timeparse"
	"github.com/jedafe/japa/internal/valkey"
)

const (
	defaultPageSize int32 = 50
	maxPageSize     int32 = 200
)

// deleteByID is a shared helper for DELETE handlers that follow the pattern:
// require user → require platform → parse UUID → call delete function → return.
func deleteByID(w http.ResponseWriter, r *http.Request, entityName string, deleteFn func(ctx context.Context, id pgtype.UUID) error) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+entityName+" ID")
		return
	}

	if err := deleteFn(r.Context(), id); err != nil {
		slog.Error("deleting "+entityName, "error", err)
		handleServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// Handlers holds dependencies for spam admin HTTP handlers.
type Handlers struct {
	Store *Store
	rdb   *valkey.Client
}

// NewHandlers creates Handlers with the given Store.
func NewHandlers(store *Store) *Handlers {
	return &Handlers{Store: store}
}

// NewHandlersWithCache creates Handlers with a Valkey client for censored-words
// cache invalidation after admin mutations (BUG-002).
func NewHandlersWithCache(store *Store, rdb *valkey.Client) *Handlers {
	return &Handlers{Store: store, rdb: rdb}
}

// invalidateCensoredCache is a best-effort cache invalidation helper. It logs
// failures but never returns an error to the caller — a stale cache (1h TTL)
// is preferable to failing an otherwise-successful admin mutation.
func (h *Handlers) invalidateCensoredCache(ctx context.Context) {
	if h.rdb == nil {
		return
	}

	if err := InvalidateCensoredWordsCache(ctx, h.rdb); err != nil {
		slog.Warn("failed to invalidate censored words cache after mutation", "error", err)
	}
}

// --- helpers ---

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

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n)
}

func parseCursorCreatedAt(r *http.Request) pgtype.Timestamptz {
	s := r.URL.Query().Get("cursor")
	if s == "" {
		return pgtype.Timestamptz{}
	}

	ts, err := timeparse.ParseTimestamptz(s)
	if err != nil {
		return pgtype.Timestamptz{}
	}

	return ts
}

// parsePageOffset parses the page_offset query param, falling back to
// page-based offset (page=N → offset = N * page_size) when page_offset is
// absent. Used by admin DataTable server-side pagination (BUG-010).
func parsePageOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		if ps := r.URL.Query().Get("page"); ps != "" {
			if p, err := strconv.ParseInt(ps, 10, 32); err == nil && p >= 0 {
				return int32(p) * parsePageSize(r)
			}
		}

		return 0
	}

	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil || v < 0 {
		return 0
	}

	return int32(v)
}

// parseOptionalStringParam returns a *string for the given query param, or nil
// when the param is absent. Used for optional filter params like block_type.
func parseOptionalStringParam(r *http.Request, key string) *string {
	s := r.URL.Query().Get(key)
	if s == "" {
		return nil
	}

	return &s
}

func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
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
// Returns offset-paginated IP blocks with total/total_pages for DataTable
// server-side pagination (BUG-010). Supports optional block_type filter.
func (h *Handlers) ListIPBlocks(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)
	blockType := parseOptionalStringParam(r, "block_type")

	blocks, err := h.Store.ListIPBlocksPaginated(r.Context(), limit, offset, blockType)
	if err != nil {
		slog.Error("listing ip blocks", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list IP blocks")

		return
	}

	count, err := h.Store.CountIPBlocks(r.Context(), blockType)
	if err != nil {
		slog.Error("counting ip blocks", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count IP blocks")

		return
	}

	totalPages := int32(1)
	if limit > 0 {
		totalPages = (count + limit - 1) / limit
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        blocks,
		"total":       count,
		"page":        offset / limit,
		"total_pages": totalPages,
	})
}

// CreateIPBlock handles POST /admin/spam/ip-blocks.
func (h *Handlers) CreateIPBlock(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.CIDR == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_cidr", "cidr is required")
		return
	}

	if input.BlockType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_block_type", "block_type is required")
		return
	}

	if !IsValidBlockType(input.BlockType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_block_type", "block_type must be one of: full, registration, posting, api")
		return
	}

	var expiresAt pgtype.Timestamptz
	if input.ExpiresAt != "" {
		if err := expiresAt.Scan(input.ExpiresAt); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_expires_at", "expires_at must be a valid timestamp")
			return
		}
	}

	block, err := h.Store.CreateIPBlock(r.Context(), CreateIPBlockInput{

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

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": block})
}

// UpdateIPBlock handles PATCH /admin/spam/ip-blocks/{id}.
func (h *Handlers) UpdateIPBlock(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid IP block ID")
		return
	}

	var input struct {
		CIDR      *string `json:"cidr"`
		BlockType *string `json:"block_type"`
		Reason    *string `json:"reason"`
		ExpiresAt *string `json:"expires_at"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.BlockType != nil && !IsValidBlockType(*input.BlockType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_block_type", "block_type must be one of: full, registration, posting, api")
		return
	}

	var expiresAt *pgtype.Timestamptz

	if input.ExpiresAt != nil && *input.ExpiresAt != "" {
		ts, err := timeparse.ParseTimestamptz(*input.ExpiresAt)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_expires_at", "expires_at must be a valid timestamp")
			return
		}

		expiresAt = &ts
	}

	block, err := h.Store.UpdateIPBlock(r.Context(), id, UpdateIPBlockInput{
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": block})
}

// DeleteIPBlock handles DELETE /admin/spam/ip-blocks/{id}.
func (h *Handlers) DeleteIPBlock(w http.ResponseWriter, r *http.Request) {
	deleteByID(w, r, "IP block", func(ctx context.Context, id pgtype.UUID) error {
		return h.Store.DeleteIPBlock(ctx, id)
	})
}

// ---------------------------------------------------------------------------
// Censored Word Handlers
// ---------------------------------------------------------------------------

// ListCensoredWords handles GET /admin/spam/censored-words.
// Returns offset-paginated censored words with total/total_pages for DataTable
// server-side pagination (BUG-010). Supports optional action filter.
func (h *Handlers) ListCensoredWords(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)
	action := parseOptionalStringParam(r, "action")

	words, err := h.Store.ListCensoredWordsPaginated(r.Context(), limit, offset, action)
	if err != nil {
		slog.Error("listing censored words", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list censored words")

		return
	}

	count, err := h.Store.CountCensoredWords(r.Context(), action)
	if err != nil {
		slog.Error("counting censored words", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count censored words")

		return
	}

	totalPages := int32(1)
	if limit > 0 {
		totalPages = (count + limit - 1) / limit
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        words,
		"total":       count,
		"page":        offset / limit,
		"total_pages": totalPages,
	})
}

// CreateCensoredWord handles POST /admin/spam/censored-words.
func (h *Handlers) CreateCensoredWord(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	var input struct {
		Pattern     string  `json:"pattern"`
		Action      string  `json:"action"`
		Replacement *string `json:"replacement"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Pattern == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_pattern", "pattern is required")
		return
	}

	if input.Action == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_action", "action is required")
		return
	}

	if !IsValidCensorAction(input.Action) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_action", "action must be one of: block, replace, flag")
		return
	}

	if input.Action == CensorActionReplace && (input.Replacement == nil || *input.Replacement == "") {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_replacement", "replacement is required when action is replace")
		return
	}

	word, err := h.Store.CreateCensoredWord(r.Context(), CreateCensoredWordInput{

		Pattern:     input.Pattern,
		Action:      input.Action,
		Replacement: input.Replacement,
	})
	if err != nil {
		slog.Error("creating censored word", "error", err)
		handleServiceError(w, err)

		return
	}

	h.invalidateCensoredCache(r.Context())

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": word})
}

// UpdateCensoredWord handles PATCH /admin/spam/censored-words/{id}.
func (h *Handlers) UpdateCensoredWord(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid censored word ID")
		return
	}

	var input struct {
		Pattern     *string `json:"pattern"`
		Action      *string `json:"action"`
		Replacement *string `json:"replacement"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Action != nil && !IsValidCensorAction(*input.Action) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_action", "action must be one of: block, replace, flag")
		return
	}

	if input.Action != nil && *input.Action == CensorActionReplace && (input.Replacement == nil || *input.Replacement == "") {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_replacement", "replacement is required when action is replace")
		return
	}

	word, err := h.Store.UpdateCensoredWord(r.Context(), id, UpdateCensoredWordInput{
		Pattern:     input.Pattern,
		Action:      input.Action,
		Replacement: input.Replacement,
	})
	if err != nil {
		slog.Error("updating censored word", "error", err)
		handleServiceError(w, err)

		return
	}

	h.invalidateCensoredCache(r.Context())

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": word})
}

// DeleteCensoredWord handles DELETE /admin/spam/censored-words/{id}.
func (h *Handlers) DeleteCensoredWord(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid censored word ID")
		return
	}

	if err := h.Store.DeleteCensoredWord(r.Context(), id); err != nil {
		slog.Error("deleting censored word", "error", err)
		handleServiceError(w, err)

		return
	}

	h.invalidateCensoredCache(r.Context())

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// User Block Handlers
// ---------------------------------------------------------------------------

// ListUserBlocks handles GET /admin/spam/user-blocks.
// Returns offset-paginated user blocks with total/total_pages for DataTable
// server-side pagination (BUG-010). Platform-scoped per JEOS §9.
func (h *Handlers) ListUserBlocks(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	blocks, err := h.Store.ListAllUserBlocksPaginated(r.Context(), limit, offset)
	if err != nil {
		slog.Error("listing user blocks", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list user blocks")

		return
	}

	count, err := h.Store.CountAllUserBlocks(r.Context())
	if err != nil {
		slog.Error("counting user blocks", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count user blocks")

		return
	}

	totalPages := int32(1)
	if limit > 0 {
		totalPages = (count + limit - 1) / limit
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        blocks,
		"total":       count,
		"page":        offset / limit,
		"total_pages": totalPages,
	})
}

// DeleteUserBlock handles DELETE /admin/spam/user-blocks/{blockerId}/{blockedId}.
func (h *Handlers) DeleteUserBlock(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	blockerID, err := parseUUIDParam(r, "blockerId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_blocker_id", "Invalid blocker ID")
		return
	}

	blockedID, err := parseUUIDParam(r, "blockedId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_blocked_id", "Invalid blocked ID")
		return
	}

	if err := h.Store.DeleteUserBlock(r.Context(), blockerID, blockedID); err != nil {
		slog.Error("deleting user block", "error", err)
		handleServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}
