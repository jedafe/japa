// Package professional — HTTP handlers for the Professional Profiles system.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package professional

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/tenant"
)

// listProfileSubEntities is a shared helper for listing sub-entities of a profile
// (relocations, documents, verifiers). All follow the same pattern: require auth,
// parse profile ID, paginate, call list function, return results.
func listProfileSubEntities(w http.ResponseWriter, r *http.Request, h *Handlers, listFn func(ctx context.Context, profileID, tenantID pgtype.UUID, limit, offset int32) ([]any, error)) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	items, err := listFn(r.Context(), profileID, tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

const defaultPageSize int32 = 20

// entityTypeProfessionalProfile matches follows.EntityTypeProfessionalProfile.
const entityTypeProfessionalProfile = "professional_profile"

// FollowsService is the subset of follows.Service the profile follow handler needs.
type FollowsService interface {
	ToggleFollow(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error)
}

// Handlers holds dependencies for professional profile HTTP handlers.
type Handlers struct {
	Service *Service
	RBAC    *rbac.RBAC
	follows FollowsService
}

// NewHandlers creates Handlers with the given Service and RBAC resolver.
func NewHandlers(service *Service, rbacResolver *rbac.RBAC, followsService FollowsService) *Handlers {
	return &Handlers{Service: service, RBAC: rbacResolver, follows: followsService}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrAlreadyExists), errors.Is(err, errpkg.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case errors.Is(err, errpkg.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "not_member", "User is not a member of this profile")
	case errors.Is(err, errpkg.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Invalid or expired invitation")
	case errors.Is(err, errpkg.ErrExpired):
		httpx.WriteError(w, http.StatusGone, "expired", "Invitation has expired")
	case errors.Is(err, ErrUpgradeToBusinessRequired):
		httpx.WriteError(w, http.StatusForbidden, "upgrade_to_business_required", "You must upgrade to a business profile to create professional profiles")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > MaxPageSize {
		return MaxPageSize
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// parsePageSizeOffset extracts the page_offset query parameter.
func parsePageSizeOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// checkProfileOwnerOrAdmin verifies the user owns the profile or has the
// professional_profile.edit_any permission. Returns true if authorized.
// If not authorized, writes a 403 response and returns false.
func (h *Handlers) checkProfileOwnerOrAdmin(w http.ResponseWriter, r *http.Request, profileID pgtype.UUID, user *auth.AuthenticatedUser, tenantID pgtype.UUID) bool {
	isOwner, err := h.Service.IsOwner(r.Context(), profileID, user.UserID, tenantID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to check ownership")
		return false
	}

	if isOwner {
		return true
	}

	// Check for admin-level edit permission.
	if h.RBAC != nil && h.RBAC.HasPermission(r.Context(), user.UserID, tenantID, "professional_profile.edit_any") {
		return true
	}

	httpx.WriteError(w, http.StatusForbidden, "forbidden", "You do not own this profile")

	return false
}

// ---------------------------------------------------------------------------
// Profile Type Definition Handlers
// ---------------------------------------------------------------------------

// CreateProfessionalProfileType handles POST /api/v1/professional-profiles/types.
func (h *Handlers) CreateProfessionalProfileType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Name         string `json:"name"`
		Slug         string `json:"slug"`
		Icon         string `json:"icon"`
		FieldGroupID string `json:"field_group_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	if input.Slug == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	var fieldGroupID pgtype.UUID
	if input.FieldGroupID != "" {
		if err := fieldGroupID.Scan(input.FieldGroupID); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_field_group_id", "Invalid field_group_id")
			return
		}
	}

	result, err := h.Service.CreateProfessionalProfileType(r.Context(), tenantID, CreateProfessionalProfileTypeInput{
		Name:         input.Name,
		Slug:         input.Slug,
		Icon:         input.Icon,
		FieldGroupID: fieldGroupID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// ListProfessionalProfileTypes handles GET /api/v1/professional-profiles/types.
func (h *Handlers) ListProfessionalProfileTypes(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	types, err := h.Service.ListProfessionalProfileTypes(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// GetProfessionalProfileType handles GET /api/v1/professional-profiles/types/{slug}.
func (h *Handlers) GetProfessionalProfileType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	result, err := h.Service.GetProfessionalProfileTypeBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// UpdateProfessionalProfileType handles PATCH /api/v1/professional-profiles/types/{id}.
func (h *Handlers) UpdateProfessionalProfileType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile type ID")
		return
	}

	var input struct {
		Name         string `json:"name"`
		Icon         string `json:"icon"`
		FieldGroupID string `json:"field_group_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	var fieldGroupID pgtype.UUID
	if input.FieldGroupID != "" {
		if err := fieldGroupID.Scan(input.FieldGroupID); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_field_group_id", "Invalid field_group_id")
			return
		}
	}

	result, err := h.Service.UpdateProfessionalProfileType(r.Context(), tenantID, id, UpdateProfessionalProfileTypeInput{
		Name:         input.Name,
		Icon:         input.Icon,
		FieldGroupID: fieldGroupID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// DeleteProfessionalProfileType handles DELETE /api/v1/professional-profiles/types/{id}.
func (h *Handlers) DeleteProfessionalProfileType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile type ID")
		return
	}

	if err := h.Service.DeleteProfessionalProfileType(r.Context(), tenantID, id); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Professional Profile CRUD Handlers
// ---------------------------------------------------------------------------

// CreateProfessionalProfile handles POST /api/v1/professional-profiles.
func (h *Handlers) CreateProfessionalProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	input, ok := decodeCreateProfileInput(w, r)
	if !ok {
		return
	}

	typeID, ok := parseRequiredUUID(w, input.TypeID, "type_id")
	if !ok {
		return
	}

	logoMediaID, ok := parseOptionalUUID(w, input.LogoMediaID, "logo_media_id")
	if !ok {
		return
	}

	headerMediaID, ok := parseOptionalUUID(w, input.HeaderMediaID, "header_media_id")
	if !ok {
		return
	}

	fieldValues := input.FieldValues
	if fieldValues == "" {
		fieldValues = "{}"
	}

	result, err := h.Service.CreateProfessionalProfile(r.Context(), tenantID, user.UserID, CreateProfessionalProfileInput{
		TypeID:           typeID,
		Name:             input.Name,
		Slug:             input.Slug,
		Tagline:          input.Tagline,
		Description:      input.Description,
		LogoMediaID:      logoMediaID,
		HeaderMediaID:    headerMediaID,
		WebsiteURL:       input.WebsiteURL,
		VerificationType: input.VerificationType,
		FieldValues:      fieldValues,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// createProfileInput is the decoded JSON body for CreateProfessionalProfile.
type createProfileInput struct {
	TypeID           string `json:"type_id"`
	Name             string `json:"name"`
	Slug             string `json:"slug"`
	Tagline          string `json:"tagline"`
	Description      string `json:"description"`
	LogoMediaID      string `json:"logo_media_id"`
	HeaderMediaID    string `json:"header_media_id"`
	WebsiteURL       string `json:"website_url"`
	VerificationType string `json:"verification_type"`
	FieldValues      string `json:"field_values"`
}

// decodeCreateProfileInput decodes and validates the create profile request body.
func decodeCreateProfileInput(w http.ResponseWriter, r *http.Request) (createProfileInput, bool) {
	var input createProfileInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return input, false
	}

	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return input, false
	}

	if input.Slug == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return input, false
	}

	if input.TypeID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_type_id", "type_id is required")
		return input, false
	}

	return input, true
}

// parseRequiredUUID parses a required UUID string parameter.
func parseRequiredUUID(w http.ResponseWriter, value, fieldName string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_"+fieldName, "Invalid "+fieldName)
		return pgtype.UUID{}, false
	}

	return id, true
}

// parseOptionalUUID parses an optional UUID string parameter. Returns zero UUID if empty.
func parseOptionalUUID(w http.ResponseWriter, value, fieldName string) (pgtype.UUID, bool) {
	if value == "" {
		return pgtype.UUID{}, true
	}

	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_"+fieldName, "Invalid "+fieldName+" format")
		return pgtype.UUID{}, false
	}

	return id, true
}

// ListProfessionalProfiles handles GET /api/v1/professional-profiles.
func (h *Handlers) ListProfessionalProfiles(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	// Filter by owner or type.
	ownerID := r.URL.Query().Get("owner_user_id")
	typeID := r.URL.Query().Get("type_id")

	if ownerID != "" {
		var oid pgtype.UUID
		if err := oid.Scan(ownerID); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_owner_user_id", "Invalid owner_user_id")
			return
		}

		profiles, err := h.Service.ListProfessionalProfilesByOwner(r.Context(), oid, tenantID, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": profiles})

		return
	}

	if typeID != "" {
		var tid pgtype.UUID
		if err := tid.Scan(typeID); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_type_id", "Invalid type_id")
			return
		}

		profiles, err := h.Service.ListProfessionalProfilesByType(r.Context(), tid, tenantID, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": profiles})

		return
	}

	// Default: list by owner (the authenticated user).
	user := auth.FromContext(r.Context())
	if user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	profiles, err := h.Service.ListProfessionalProfilesByOwner(r.Context(), user.UserID, tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": profiles})
}

// GetProfessionalProfile handles GET /api/v1/professional-profiles/{slug}.
func (h *Handlers) GetProfessionalProfile(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	result, err := h.Service.GetProfessionalProfileBySlug(r.Context(), slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// UpdateProfessionalProfile handles PATCH /api/v1/professional-profiles/{id}.
func (h *Handlers) UpdateProfessionalProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	if !h.checkProfileOwnerOrAdmin(w, r, id, user, tenantID) {
		return
	}

	var input struct {
		Name             string `json:"name"`
		Tagline          string `json:"tagline"`
		Description      string `json:"description"`
		LogoMediaID      string `json:"logo_media_id"`
		HeaderMediaID    string `json:"header_media_id"`
		WebsiteURL       string `json:"website_url"`
		VerificationType string `json:"verification_type"`
		Status           string `json:"status"`
		FieldValues      string `json:"field_values"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	var logoMediaID, headerMediaID pgtype.UUID
	if input.LogoMediaID != "" {
		if err := logoMediaID.Scan(input.LogoMediaID); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_logo_media_id", "Invalid logo media ID format")
			return
		}
	}

	if input.HeaderMediaID != "" {
		if err := headerMediaID.Scan(input.HeaderMediaID); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_header_media_id", "Invalid header media ID format")
			return
		}
	}

	result, err := h.Service.UpdateProfessionalProfile(r.Context(), id, tenantID, UpdateProfessionalProfileInput{
		Name:             input.Name,
		Tagline:          input.Tagline,
		Description:      input.Description,
		LogoMediaID:      logoMediaID,
		HeaderMediaID:    headerMediaID,
		WebsiteURL:       input.WebsiteURL,
		VerificationType: input.VerificationType,
		Status:           input.Status,
		FieldValues:      input.FieldValues,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// DeleteProfessionalProfile handles DELETE /api/v1/professional-profiles/{id}.
func (h *Handlers) DeleteProfessionalProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	if !h.checkProfileOwnerOrAdmin(w, r, id, user, tenantID) {
		return
	}

	if err := h.Service.DeleteProfessionalProfile(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Member Management Handlers
// ---------------------------------------------------------------------------

// AddMember handles POST /api/v1/professional-profiles/{id}/members.
func (h *Handlers) AddMember(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	if !h.checkProfileOwnerOrAdmin(w, r, profileID, user, tenantID) {
		return
	}

	var input struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.UserID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_user_id", "user_id is required")
		return
	}

	var memberUserID pgtype.UUID
	if err := memberUserID.Scan(input.UserID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id")
		return
	}

	if err := h.Service.AddMember(r.Context(), profileID, user.UserID, AddMemberInput{
		UserID: memberUserID,
		Role:   input.Role,
	}); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"status": "added"}})
}

// RemoveMember handles DELETE /api/v1/professional-profiles/{id}/members/{userId}.
func (h *Handlers) RemoveMember(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	userID, err := httpx.ParseUUIDParam(r, "userId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID")
		return
	}

	if err := h.Service.RemoveMember(r.Context(), profileID, userID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// ListMembers handles GET /api/v1/professional-profiles/{id}/members.
func (h *Handlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	members, err := h.Service.ListMembers(r.Context(), profileID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": members})
}

// UpdateMemberRole handles PATCH /api/v1/professional-profiles/{id}/members/{userId}.
func (h *Handlers) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	userID, err := httpx.ParseUUIDParam(r, "userId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID")
		return
	}

	var input struct {
		Role string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Role == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_role", "role is required")
		return
	}

	if err := h.Service.UpdateMemberRole(r.Context(), profileID, userID, UpdateMemberRoleInput{Role: input.Role}); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// ---------------------------------------------------------------------------
// Invitation Management Handlers
// ---------------------------------------------------------------------------

// CreateInvitation handles POST /api/v1/professional-profiles/{id}/invitations.
func (h *Handlers) CreateInvitation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	if !h.checkProfileOwnerOrAdmin(w, r, profileID, user, tenantID) {
		return
	}

	var input struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Email == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_email", "email is required")
		return
	}

	inv, err := h.Service.CreateInvitation(r.Context(), profileID, user.UserID, CreateInvitationInput{
		Email: input.Email,
		Role:  input.Role,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": inv})
}

// ListInvitations handles GET /api/v1/professional-profiles/{id}/invitations.
func (h *Handlers) ListInvitations(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	invitations, err := h.Service.ListInvitations(r.Context(), profileID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": invitations})
}

// AcceptInvitation handles POST /api/v1/professional-profiles/invitations/{token}/accept.
func (h *Handlers) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	token := chi.URLParam(r, "token")
	if token == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_token", "token is required")
		return
	}

	if err := h.Service.AcceptInvitation(r.Context(), token, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "accepted"}})
}

// ---------------------------------------------------------------------------
// Authorship Context Handler
// ---------------------------------------------------------------------------

// GetAuthorContext handles GET /api/v1/professional-profiles/{id}/author-context.
func (h *Handlers) GetAuthorContext(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	authorCtx, err := h.Service.GetAuthorContext(r.Context(), profileID, user.UserID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": authorCtx})
}

// ---------------------------------------------------------------------------
// Relocation Handlers
// ---------------------------------------------------------------------------

// ListRelocations handles GET /api/v1/professional-profiles/{id}/relocations.
func (h *Handlers) ListRelocations(w http.ResponseWriter, r *http.Request) {
	listProfileSubEntities(w, r, h, func(ctx context.Context, profileID, tenantID pgtype.UUID, limit, offset int32) ([]any, error) {
		relocations, err := h.Service.ListRelocations(ctx, profileID, tenantID, limit, offset)
		if err != nil {
			return nil, err
		}

		out := make([]any, len(relocations))
		for i := range relocations {
			out[i] = relocations[i]
		}

		return out, nil
	})
}

// ListDocuments handles GET /api/v1/professional-profiles/{id}/documents.
func (h *Handlers) ListDocuments(w http.ResponseWriter, r *http.Request) {
	listProfileSubEntities(w, r, h, func(ctx context.Context, profileID, tenantID pgtype.UUID, limit, offset int32) ([]any, error) {
		documents, err := h.Service.ListDocuments(ctx, profileID, tenantID, limit, offset)
		if err != nil {
			return nil, err
		}

		out := make([]any, len(documents))
		for i := range documents {
			out[i] = documents[i]
		}

		return out, nil
	})
}

// ListVerifiers handles GET /api/v1/professional-profiles/{id}/verifiers.
func (h *Handlers) ListVerifiers(w http.ResponseWriter, r *http.Request) {
	listProfileSubEntities(w, r, h, func(ctx context.Context, profileID, tenantID pgtype.UUID, limit, offset int32) ([]any, error) {
		verifiers, err := h.Service.ListVerifiers(ctx, profileID, tenantID, limit, offset)
		if err != nil {
			return nil, err
		}

		out := make([]any, len(verifiers))
		for i := range verifiers {
			out[i] = verifiers[i]
		}

		return out, nil
	})
}

// CreateRelocation handles POST /api/v1/professional-profiles/{id}/relocations.
func (h *Handlers) CreateRelocation(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	var input struct {
		Country string `json:"country"`
		City    string `json:"city"`
		Status  string `json:"status"`
		Year    int32  `json:"year"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Country == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_country", "country is required")
		return
	}

	relocation, err := h.Service.CreateRelocation(r.Context(), tenantID, profileID, CreateRelocationInput{
		Country: input.Country,
		City:    input.City,
		Status:  input.Status,
		Year:    input.Year,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": relocation})
}

// UpdateRelocation handles PATCH /api/v1/professional-profiles/relocations/{relocationId}.
func (h *Handlers) UpdateRelocation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	relocationID, err := httpx.ParseUUIDParam(r, "relocationId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_relocation_id", "Invalid relocation ID")
		return
	}

	profileID, err := h.Service.GetRelocationProfileID(r.Context(), relocationID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !h.checkProfileOwnerOrAdmin(w, r, profileID, user, tenantID) {
		return
	}

	var input struct {
		Country string `json:"country"`
		City    string `json:"city"`
		Status  string `json:"status"`
		Year    int32  `json:"year"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	relocation, err := h.Service.UpdateRelocation(r.Context(), relocationID, tenantID, UpdateRelocationInput{
		Country: input.Country,
		City:    input.City,
		Status:  input.Status,
		Year:    input.Year,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": relocation})
}

// DeleteRelocation handles DELETE /api/v1/professional-profiles/relocations/{relocationId}.
func (h *Handlers) DeleteRelocation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	relocationID, err := httpx.ParseUUIDParam(r, "relocationId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_relocation_id", "Invalid relocation ID")
		return
	}

	profileID, err := h.Service.GetRelocationProfileID(r.Context(), relocationID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !h.checkProfileOwnerOrAdmin(w, r, profileID, user, tenantID) {
		return
	}

	if err := h.Service.DeleteRelocation(r.Context(), relocationID, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
}

// ---------------------------------------------------------------------------
// Document Handlers
// ---------------------------------------------------------------------------

// CreateDocument handles POST /api/v1/professional-profiles/{id}/documents.
func (h *Handlers) CreateDocument(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	var input struct {
		Title        string `json:"title"`
		DocumentType string `json:"document_type"`
		Description  string `json:"description"`
		FileURL      string `json:"file_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	if input.DocumentType == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_document_type", "document_type is required")
		return
	}

	document, err := h.Service.CreateDocument(r.Context(), tenantID, profileID, CreateDocumentInput{
		Title:        input.Title,
		DocumentType: input.DocumentType,
		Description:  input.Description,
		FileURL:      input.FileURL,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": document})
}

// UpdateDocument handles PATCH /api/v1/professional-profiles/documents/{documentId}.
func (h *Handlers) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	documentID, err := httpx.ParseUUIDParam(r, "documentId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_document_id", "Invalid document ID")
		return
	}

	profileID, err := h.Service.GetDocumentProfileID(r.Context(), documentID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !h.checkProfileOwnerOrAdmin(w, r, profileID, user, tenantID) {
		return
	}

	var input struct {
		Title        string `json:"title"`
		DocumentType string `json:"document_type"`
		Description  string `json:"description"`
		FileURL      string `json:"file_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	document, err := h.Service.UpdateDocument(r.Context(), documentID, tenantID, UpdateDocumentInput{
		Title:        input.Title,
		DocumentType: input.DocumentType,
		Description:  input.Description,
		FileURL:      input.FileURL,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": document})
}

// DeleteDocument handles DELETE /api/v1/professional-profiles/documents/{documentId}.
func (h *Handlers) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	documentID, err := httpx.ParseUUIDParam(r, "documentId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_document_id", "Invalid document ID")
		return
	}

	profileID, err := h.Service.GetDocumentProfileID(r.Context(), documentID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !h.checkProfileOwnerOrAdmin(w, r, profileID, user, tenantID) {
		return
	}

	if err := h.Service.DeleteDocument(r.Context(), documentID, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
}

// ---------------------------------------------------------------------------
// Verifier Handlers
// ---------------------------------------------------------------------------

// CreateVerifier handles POST /api/v1/professional-profiles/{id}/verifiers.
func (h *Handlers) CreateVerifier(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	if !h.checkProfileOwnerOrAdmin(w, r, profileID, user, tenantID) {
		return
	}

	var input struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.UserID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_user_id", "user_id is required")
		return
	}

	var verifierUserID pgtype.UUID
	if err := verifierUserID.Scan(input.UserID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id")
		return
	}

	verifier, err := h.Service.CreateVerifier(r.Context(), tenantID, profileID, CreateVerifierInput{
		UserID: verifierUserID,
		Role:   input.Role,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": verifier})
}

// DeleteVerifier handles DELETE /api/v1/professional-profiles/verifiers/{verifierId}.
func (h *Handlers) DeleteVerifier(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	verifierID, err := httpx.ParseUUIDParam(r, "verifierId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_verifier_id", "Invalid verifier ID")
		return
	}

	profileID, err := h.Service.GetVerifierProfileID(r.Context(), verifierID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !h.checkProfileOwnerOrAdmin(w, r, profileID, user, tenantID) {
		return
	}

	if err := h.Service.DeleteVerifier(r.Context(), verifierID, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// ToggleFollow handles POST/DELETE /api/v1/profiles/{id}/follow.
func (h *Handlers) ToggleFollow(w http.ResponseWriter, r *http.Request) {
	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	following, err := h.follows.ToggleFollow(r.Context(), tenantID, user.UserID, entityTypeProfessionalProfile, profileID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to toggle follow")
		return
	}

	if r.Method == http.MethodDelete {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"following": false}})
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"following": following}})
}

// ─── P5.6 Professional Profile Extras ─────────────────────────────────────

// VerifyProfile handles POST /admin/profiles/:id/verify (P5.6).
func (h *Handlers) VerifyProfile(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	_, err = h.Service.db.Writer.Exec(r.Context(),
		`UPDATE professional_profiles SET verified = true, updated_at = now()
		 WHERE id = $1 AND tenant_id = $2`, profileID, tenantID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify profile")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": profileID.String(), "verified": true}})
}

// UnverifyProfile handles DELETE /admin/profiles/:id/verify (P5.6).
func (h *Handlers) UnverifyProfile(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	_, err = h.Service.db.Writer.Exec(r.Context(),
		`UPDATE professional_profiles SET verified = false, updated_at = now()
		 WHERE id = $1 AND tenant_id = $2`, profileID, tenantID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to unverify profile")
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// ListProfileContent handles GET /profiles/:id/content (P5.6).
func (h *Handlers) ListProfileContent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	limit := int32(parsePageSize(r))
	offset := int32(parsePageSizeOffset(r))

	content, err := h.Service.ListProfileContent(r.Context(), tenantID, profileID, limit, offset)
	if err != nil {
		slog.Error("professional: list profile content", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list profile content")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": content})
}

// InviteMember handles POST /profiles/:id/members/invite (P5.6).
func (h *Handlers) InviteMember(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	profileID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	var input struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Email == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_email", "Email is required")
		return
	}

	// Create invitation via raw SQL
	token := fmt.Sprintf("inv_%d", time.Now().UnixNano())

	_, err = h.Service.db.Writer.Exec(r.Context(),
		`INSERT INTO professional_profile_invitations (profile_id, email, role, invited_by, token, expires_at)
		 VALUES ($1, $2, $3, $4, $5, now() + interval '7 days')`,
		profileID, input.Email, input.Role, pgtype.UUID{}, token)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create invitation")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{
		"profile_id": profileID.String(), "email": input.Email, "token": token,
	}})
}

// AcceptProfileInvitation handles POST /profiles/invitations/accept (P5.6).
func (h *Handlers) AcceptProfileInvitation(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Token == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_token", "Token is required")
		return
	}

	_ = tenantID

	// Accept invitation via raw SQL
	_, err := h.Service.db.Writer.Exec(r.Context(),
		`UPDATE professional_profile_invitations SET accepted_at = now()
		 WHERE token = $1 AND accepted_at IS NULL AND expires_at > now()`, input.Token)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to accept invitation")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"accepted": true}})
}
