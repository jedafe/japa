package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
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
	IsActive    bool   `json:"is_active"`
}

var literalKeyRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ListLiterals handles GET /admin/literals.
func (h *Handlers) ListLiterals(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query().Get("q")
	sort := r.URL.Query().Get("sort")
	dir := r.URL.Query().Get("dir")

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize < 1 {
		pageSize = 25
	}
	if pageSize > 1000 {
		pageSize = 1000
	}

	var isActive *bool
	if v := r.URL.Query().Get("is_active"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			isActive = &b
		}
	}

	rows, err := h.Queries.ListTemplateLiterals(r.Context(), gen.ListTemplateLiteralsParams{
		TenantID:   tenantID,
		Search:     q,
		IsActive:   isActive,
		Sort:       sort,
		Dir:        dir,
		PageOffset: int32((page - 1) * pageSize),
		PageLimit:  int32(pageSize),
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to load literals")
		return
	}

	total, err := h.Queries.CountTemplateLiterals(r.Context(), gen.CountTemplateLiteralsParams{
		TenantID: tenantID,
		Search:   q,
		IsActive: isActive,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to count literals")
		return
	}

	totalPages := (total + int64(pageSize) - 1) / int64(pageSize)
	if total == 0 {
		totalPages = 0
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        rows,
		"total":       total,
		"total_pages": totalPages,
	})
}

// CreateLiteral handles POST /admin/literals.
func (h *Handlers) CreateLiteral(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var req LiteralRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	if req.Key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Key is required")
		return
	}
	if !literalKeyRegex.MatchString(req.Key) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_key", "Key must start with a letter or underscore and contain only letters, numbers, and underscores")
		return
	}

	if req.Kind == "" {
		req.Kind = "expression"
	}
	if req.Kind != "expression" && req.Kind != "static" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_kind", "Kind must be 'expression' or 'static'")
		return
	}

	if req.Kind == "expression" {
		if _, err := expr.Parse(req.Value); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_expression", err.Error())
			return
		}
	}

	_, err := h.Queries.GetTemplateLiteralByKey(r.Context(), gen.GetTemplateLiteralByKeyParams{
		TenantID: tenantID,
		Key:      req.Key,
	})
	if err == nil {
		errpkg.WriteError(w, http.StatusConflict, "duplicate_key", "A literal with this key already exists")
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to check literal key")
		return
	}

	lit, err := h.Queries.CreateTemplateLiteral(r.Context(), gen.CreateTemplateLiteralParams{
		TenantID:    tenantID,
		Key:         req.Key,
		Kind:        req.Kind,
		Value:       req.Value,
		Fallback:    ptrString(req.Fallback),
		Label:       ptrString(req.Label),
		Description: ptrString(req.Description),
		Category:    ptrString(req.Category),
		IsActive:    req.IsActive,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to create literal")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": lit})
}

// GetLiteral handles GET /admin/literals/{id}.
func (h *Handlers) GetLiteral(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid literal ID")
		return
	}

	lit, err := h.Queries.GetTemplateLiteral(r.Context(), gen.GetTemplateLiteralParams{
		TenantID: tenantID,
		ID:       pgtype.UUID{Bytes: id, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Literal not found")
			return
		}
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to load literal")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": lit})
}

// UpdateLiteral handles PATCH /admin/literals/{id}.
func (h *Handlers) UpdateLiteral(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid literal ID")
		return
	}

	existing, err := h.Queries.GetTemplateLiteral(r.Context(), gen.GetTemplateLiteralParams{
		TenantID: tenantID,
		ID:       pgtype.UUID{Bytes: id, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Literal not found")
			return
		}
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to load literal")
		return
	}

	if existing.IsSystem {
		errpkg.WriteError(w, http.StatusForbidden, "system_literal", "Cannot modify system literals")
		return
	}

	var req LiteralRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	kind := req.Kind
	if kind == "" {
		kind = existing.Kind
	}
	if kind != "expression" && kind != "static" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_kind", "Kind must be 'expression' or 'static'")
		return
	}

	value := req.Value
	if value == "" {
		value = existing.Value
	}

	if kind == "expression" {
		if _, err := expr.Parse(value); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_expression", err.Error())
			return
		}
	}

	fallback := req.Fallback
	if fallback == "" && existing.Fallback != nil {
		fallback = *existing.Fallback
	}
	label := req.Label
	if label == "" && existing.Label != nil {
		label = *existing.Label
	}
	description := req.Description
	if description == "" && existing.Description != nil {
		description = *existing.Description
	}
	category := req.Category
	if category == "" && existing.Category != nil {
		category = *existing.Category
	}

	lit, err := h.Queries.UpdateTemplateLiteral(r.Context(), gen.UpdateTemplateLiteralParams{
		Kind:        kind,
		Value:       value,
		Fallback:    ptrString(fallback),
		Label:       ptrString(label),
		Description: ptrString(description),
		Category:    ptrString(category),
		IsActive:    req.IsActive,
		TenantID:    tenantID,
		ID:          pgtype.UUID{Bytes: id, Valid: true},
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to update literal")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": lit})
}

// DeleteLiteral handles DELETE /admin/literals/{id}.
func (h *Handlers) DeleteLiteral(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid literal ID")
		return
	}

	existing, err := h.Queries.GetTemplateLiteral(r.Context(), gen.GetTemplateLiteralParams{
		TenantID: tenantID,
		ID:       pgtype.UUID{Bytes: id, Valid: true},
	})
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

	if err := h.Queries.DeleteTemplateLiteral(r.Context(), gen.DeleteTemplateLiteralParams{
		TenantID: tenantID,
		ID:       pgtype.UUID{Bytes: id, Valid: true},
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to delete literal")
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}


