package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/literals/expr"
)

// LiteralRequest is the body for creating or updating a template literal.
type LiteralRequest struct {
	Key         string `json:"key"`
	Kind        string `json:"kind"`
	Value       string `json:"value"`
	Fallback    string `json:"fallback"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Category    string `json:"category"`
	IsActive    *bool  `json:"is_active"`
}

const (
	literalKindExpression = "expression"
	literalKindStatic     = "static"

	defaultPageSize = int64(25)
	maxPageSize     = int64(1000)
	maxInt32Value   = int64(1<<31 - 1)

	responseKeyData       = "data"
	responseKeyTotal      = "total"
	responseKeyTotalPages = "total_pages"
)

var literalKeyRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func ptrString(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

func coalesceString(req, existing string) string {
	if req != "" {
		return req
	}

	return existing
}

// ListLiterals handles GET /admin/literals.
func (h *Handlers) ListLiterals(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query().Get("q")
	kind := r.URL.Query().Get("kind")
	sort := r.URL.Query().Get("sort")
	dir := r.URL.Query().Get("dir")

	page, pageSize := parsePageParams(r)

	var isActive *bool

	if v := r.URL.Query().Get("is_active"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			isActive = &b
		}
	}

	offset, limit := computePagination(page, pageSize)

	rows, err := h.Queries.ListTemplateLiterals(r.Context(), gen.ListTemplateLiteralsParams{

		Search:     q,
		IsActive:   isActive,
		Kind:       kind,
		Sort:       sort,
		Dir:        dir,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to load literals")
		return
	}

	total, err := h.Queries.CountTemplateLiterals(r.Context(), gen.CountTemplateLiteralsParams{

		Search:   q,
		IsActive: isActive,
		Kind:     kind,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to count literals")
		return
	}

	totalPages := (total + pageSize - 1) / pageSize
	if total == 0 {
		totalPages = 0
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		responseKeyData:       rows,
		responseKeyTotal:      total,
		responseKeyTotalPages: totalPages,
	})
}

func parsePageParams(r *http.Request) (int64, int64) {
	page, err := strconv.ParseInt(r.URL.Query().Get("page"), 10, 32)
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.ParseInt(r.URL.Query().Get("pageSize"), 10, 32)
	if err != nil || pageSize < 1 {
		pageSize = defaultPageSize
	}

	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	return page, pageSize
}

func computePagination(page, pageSize int64) (int32, int32) {
	offset := (page - 1) * pageSize
	if offset > maxInt32Value {
		offset = maxInt32Value
	}

	//nolint:gosec // offset is bounded to MaxInt32 and pageSize to maxPageSize
	return int32(offset), int32(pageSize)
}

// CreateLiteral handles POST /admin/literals.
func (h *Handlers) CreateLiteral(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	req, ok := h.parseCreateRequest(w, r)
	if !ok {
		return
	}

	if ok := h.requireUniqueLiteralKey(r.Context(), w, req.Key); !ok {
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	lit, err := h.Queries.CreateTemplateLiteral(r.Context(), gen.CreateTemplateLiteralParams{

		Key:         req.Key,
		Kind:        req.Kind,
		Value:       req.Value,
		Fallback:    ptrString(req.Fallback),
		Label:       ptrString(req.Label),
		Description: ptrString(req.Description),
		Category:    ptrString(req.Category),
		IsActive:    isActive,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to create literal")
		return
	}

	h.invalidateLiteralCache(r.Context())

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{responseKeyData: lit})
}

func (h *Handlers) parseCreateRequest(w http.ResponseWriter, r *http.Request) (LiteralRequest, bool) {
	var req LiteralRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return req, false
	}

	if req.Key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Key is required")
		return req, false
	}

	if !literalKeyRegex.MatchString(req.Key) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_key", "Key must start with a letter or underscore and contain only letters, numbers, and underscores")
		return req, false
	}

	if literals.IsReservedKey(req.Key) {
		errpkg.WriteError(w, http.StatusBadRequest, "reserved_key", "Key is a reserved built-in name")
		return req, false
	}

	if req.Kind == "" {
		req.Kind = literalKindExpression
	}

	if req.Kind != literalKindExpression && req.Kind != literalKindStatic {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_kind", "Kind must be 'expression' or 'static'")
		return req, false
	}

	if req.Kind == literalKindExpression {
		if _, err := expr.Parse(req.Value); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_expression", err.Error())
			return req, false
		}
	}

	return req, true
}

func (h *Handlers) requireUniqueLiteralKey(ctx context.Context, w http.ResponseWriter, key string) bool {
	_, err := h.Queries.GetTemplateLiteralByKey(ctx,

		key,
	)
	if err == nil {
		errpkg.WriteError(w, http.StatusConflict, "duplicate_key", "A literal with this key already exists")
		return false
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to check literal key")
		return false
	}

	return true
}

// GetLiteral handles GET /admin/literals/{id}.
func (h *Handlers) GetLiteral(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid literal ID")
		return
	}

	lit, err := h.Queries.GetTemplateLiteral(r.Context(),

		pgtype.UUID{Bytes: id, Valid: true},
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Literal not found")
			return
		}

		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to load literal")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{responseKeyData: lit})
}

// UpdateLiteral handles PATCH /admin/literals/{id}.
func (h *Handlers) UpdateLiteral(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, existing, ok := h.loadLiteralForUpdate(w, r)
	if !ok {
		return
	}

	var req LiteralRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	params, ok := h.buildUpdateParams(w, id, existing, req)
	if !ok {
		return
	}

	lit, err := h.Queries.UpdateTemplateLiteral(r.Context(), params)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to update literal")
		return
	}

	h.invalidateLiteralCache(r.Context())

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{responseKeyData: lit})
}

func (h *Handlers) loadLiteralForUpdate(w http.ResponseWriter, r *http.Request) (uuid.UUID, gen.TemplateLiteral, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid literal ID")
		return uuid.UUID{}, gen.TemplateLiteral{}, false
	}

	existing, err := h.Queries.GetTemplateLiteral(r.Context(),

		pgtype.UUID{Bytes: id, Valid: true},
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Literal not found")
			return uuid.UUID{}, gen.TemplateLiteral{}, false
		}

		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to load literal")

		return uuid.UUID{}, gen.TemplateLiteral{}, false
	}

	if existing.IsSystem {
		errpkg.WriteError(w, http.StatusForbidden, "system_literal", "Cannot modify system literals")
		return uuid.UUID{}, gen.TemplateLiteral{}, false
	}

	return id, existing, true
}

func (h *Handlers) buildUpdateParams(w http.ResponseWriter, id uuid.UUID, existing gen.TemplateLiteral, req LiteralRequest) (gen.UpdateTemplateLiteralParams, bool) {
	kind := coalesceString(req.Kind, existing.Kind)
	if kind != literalKindExpression && kind != literalKindStatic {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_kind", "Kind must be 'expression' or 'static'")
		return gen.UpdateTemplateLiteralParams{}, false
	}

	value := coalesceString(req.Value, existing.Value)
	if kind == literalKindExpression {
		if _, err := expr.Parse(value); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_expression", err.Error())
			return gen.UpdateTemplateLiteralParams{}, false
		}
	}

	fallback := coalesceString(req.Fallback, derefString(existing.Fallback))
	label := coalesceString(req.Label, derefString(existing.Label))
	description := coalesceString(req.Description, derefString(existing.Description))
	category := coalesceString(req.Category, derefString(existing.Category))

	isActive := existing.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	return gen.UpdateTemplateLiteralParams{
		Kind:        kind,
		Value:       value,
		Fallback:    ptrString(fallback),
		Label:       ptrString(label),
		Description: ptrString(description),
		Category:    ptrString(category),
		IsActive:    isActive,

		ID: pgtype.UUID{Bytes: id, Valid: true},
	}, true
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func (h *Handlers) invalidateLiteralCache(ctx context.Context) {
	if h.Catalog == nil {
		return
	}

	if err := h.Catalog.Invalidate(ctx); err != nil {
		slog.Warn("admin: failed to invalidate literal cache", "error", err)
	}
}

// DeleteLiteral handles DELETE /admin/literals/{id}.
func (h *Handlers) DeleteLiteral(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid literal ID")
		return
	}

	existing, err := h.Queries.GetTemplateLiteral(r.Context(),

		pgtype.UUID{Bytes: id, Valid: true},
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Literal not found")
			return
		}

		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to load literal")

		return
	}

	if existing.IsSystem {
		errpkg.WriteError(w, http.StatusForbidden, "system_literal", "Cannot delete system literals")
		return
	}

	if err := h.Queries.DeleteTemplateLiteral(r.Context(),

		pgtype.UUID{Bytes: id, Valid: true},
	); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to delete literal")
		return
	}

	h.invalidateLiteralCache(r.Context())

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}
