// Package professional — HTTP handlers for the Professional Profiles system.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package professional

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for professional profile HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", param, err)
	}
	return id, nil
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case strings.Contains(err.Error(), "already_exists"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case strings.Contains(err.Error(), "duplicate"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case strings.Contains(err.Error(), "not a member"):
		writeError(w, http.StatusForbidden, "not_member", "User is not a member of this profile")
	case strings.Contains(err.Error(), "invalid or expired invitation"):
		writeError(w, http.StatusNotFound, "not_found", "Invalid or expired invitation")
	case strings.Contains(err.Error(), "invitation has expired"):
		writeError(w, http.StatusGone, "expired", "Invitation has expired")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return DefaultPageSize
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return DefaultPageSize
	}
	if n > MaxPageSize {
		return MaxPageSize
	}
	return int32(n)
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
	return int32(n)
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}
	if input.Slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	var fieldGroupID pgtype.UUID
	if input.FieldGroupID != "" {
		if err := fieldGroupID.Scan(input.FieldGroupID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_field_group_id", "Invalid field_group_id")
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
	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
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
	writeJSON(w, http.StatusOK, map[string]any{"data": types})
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
		writeError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	result, err := h.Service.GetProfessionalProfileTypeBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
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

	var input struct {
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
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}
	if input.Slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}
	if input.TypeID == "" {
		writeError(w, http.StatusBadRequest, "missing_type_id", "type_id is required")
		return
	}

	var typeID pgtype.UUID
	if err := typeID.Scan(input.TypeID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_type_id", "Invalid type_id")
		return
	}

	var logoMediaID, headerMediaID pgtype.UUID
	if input.LogoMediaID != "" {
		_ = logoMediaID.Scan(input.LogoMediaID)
	}
	if input.HeaderMediaID != "" {
		_ = headerMediaID.Scan(input.HeaderMediaID)
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
	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
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
			writeError(w, http.StatusBadRequest, "invalid_owner_user_id", "Invalid owner_user_id")
			return
		}
		profiles, err := h.Service.ListProfessionalProfilesByOwner(r.Context(), oid, tenantID, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": profiles})
		return
	}

	if typeID != "" {
		var tid pgtype.UUID
		if err := tid.Scan(typeID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_type_id", "Invalid type_id")
			return
		}
		profiles, err := h.Service.ListProfessionalProfilesByType(r.Context(), tid, tenantID, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": profiles})
		return
	}

	// Default: list by owner (the authenticated user).
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	profiles, err := h.Service.ListProfessionalProfilesByOwner(r.Context(), user.UserID, tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": profiles})
}

// GetProfessionalProfile handles GET /api/v1/professional-profiles/{slug}.
func (h *Handlers) GetProfessionalProfile(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	result, err := h.Service.GetProfessionalProfileBySlug(r.Context(), slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// UpdateProfessionalProfile handles PATCH /api/v1/professional-profiles/{id}.
func (h *Handlers) UpdateProfessionalProfile(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	var logoMediaID, headerMediaID pgtype.UUID
	if input.LogoMediaID != "" {
		_ = logoMediaID.Scan(input.LogoMediaID)
	}
	if input.HeaderMediaID != "" {
		_ = headerMediaID.Scan(input.HeaderMediaID)
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
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// DeleteProfessionalProfile handles DELETE /api/v1/professional-profiles/{id}.
func (h *Handlers) DeleteProfessionalProfile(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	if err := h.Service.DeleteProfessionalProfile(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
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
	profileID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	var input struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.UserID == "" {
		writeError(w, http.StatusBadRequest, "missing_user_id", "user_id is required")
		return
	}

	var memberUserID pgtype.UUID
	if err := memberUserID.Scan(input.UserID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id")
		return
	}

	if err := h.Service.AddMember(r.Context(), profileID, user.UserID, AddMemberInput{
		UserID: memberUserID,
		Role:   input.Role,
	}); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"status": "added"}})
}

// RemoveMember handles DELETE /api/v1/professional-profiles/{id}/members/{userId}.
func (h *Handlers) RemoveMember(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	profileID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}
	userID, err := parseUUIDParam(r, "userId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID")
		return
	}

	if err := h.Service.RemoveMember(r.Context(), profileID, userID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ListMembers handles GET /api/v1/professional-profiles/{id}/members.
func (h *Handlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	profileID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	members, err := h.Service.ListMembers(r.Context(), profileID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": members})
}

// UpdateMemberRole handles PATCH /api/v1/professional-profiles/{id}/members/{userId}.
func (h *Handlers) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	profileID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}
	userID, err := parseUUIDParam(r, "userId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID")
		return
	}

	var input struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Role == "" {
		writeError(w, http.StatusBadRequest, "missing_role", "role is required")
		return
	}

	if err := h.Service.UpdateMemberRole(r.Context(), profileID, userID, UpdateMemberRoleInput{Role: input.Role}); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
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
	profileID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	var input struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Email == "" {
		writeError(w, http.StatusBadRequest, "missing_email", "email is required")
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
	writeJSON(w, http.StatusCreated, map[string]any{"data": inv})
}

// ListInvitations handles GET /api/v1/professional-profiles/{id}/invitations.
func (h *Handlers) ListInvitations(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	profileID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	invitations, err := h.Service.ListInvitations(r.Context(), profileID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": invitations})
}

// AcceptInvitation handles POST /api/v1/professional-profiles/invitations/{token}/accept.
func (h *Handlers) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "missing_token", "token is required")
		return
	}

	if err := h.Service.AcceptInvitation(r.Context(), token, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "accepted"}})
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
	profileID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	authorCtx, err := h.Service.GetAuthorContext(r.Context(), profileID, user.UserID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": authorCtx})
}

// ---------------------------------------------------------------------------
// Relocation Handlers
// ---------------------------------------------------------------------------

// ListRelocations handles GET /api/v1/professional-profiles/{id}/relocations.
func (h *Handlers) ListRelocations(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	profileID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	relocations, err := h.Service.ListRelocations(r.Context(), profileID, tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": relocations})
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
	profileID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	var input struct {
		Country string `json:"country"`
		City    string `json:"city"`
		Status  string `json:"status"`
		Year    int32  `json:"year"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Country == "" {
		writeError(w, http.StatusBadRequest, "missing_country", "country is required")
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
	writeJSON(w, http.StatusCreated, map[string]any{"data": relocation})
}

// UpdateRelocation handles PATCH /api/v1/professional-profiles/relocations/{relocationId}.
func (h *Handlers) UpdateRelocation(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	relocationID, err := parseUUIDParam(r, "relocationId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_relocation_id", "Invalid relocation ID")
		return
	}

	var input struct {
		Country string `json:"country"`
		City    string `json:"city"`
		Status  string `json:"status"`
		Year    int32  `json:"year"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
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
	writeJSON(w, http.StatusOK, map[string]any{"data": relocation})
}

// DeleteRelocation handles DELETE /api/v1/professional-profiles/relocations/{relocationId}.
func (h *Handlers) DeleteRelocation(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	relocationID, err := parseUUIDParam(r, "relocationId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_relocation_id", "Invalid relocation ID")
		return
	}

	if err := h.Service.DeleteRelocation(r.Context(), relocationID, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
}

// ---------------------------------------------------------------------------
// Document Handlers
// ---------------------------------------------------------------------------

// ListDocuments handles GET /api/v1/professional-profiles/{id}/documents.
func (h *Handlers) ListDocuments(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	profileID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	documents, err := h.Service.ListDocuments(r.Context(), profileID, tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": documents})
}

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
	profileID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	var input struct {
		Title        string `json:"title"`
		DocumentType string `json:"document_type"`
		Description  string `json:"description"`
		FileURL      string `json:"file_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}
	if input.DocumentType == "" {
		writeError(w, http.StatusBadRequest, "missing_document_type", "document_type is required")
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
	writeJSON(w, http.StatusCreated, map[string]any{"data": document})
}

// UpdateDocument handles PATCH /api/v1/professional-profiles/documents/{documentId}.
func (h *Handlers) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	documentID, err := parseUUIDParam(r, "documentId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_document_id", "Invalid document ID")
		return
	}

	var input struct {
		Title        string `json:"title"`
		DocumentType string `json:"document_type"`
		Description  string `json:"description"`
		FileURL      string `json:"file_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
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
	writeJSON(w, http.StatusOK, map[string]any{"data": document})
}

// DeleteDocument handles DELETE /api/v1/professional-profiles/documents/{documentId}.
func (h *Handlers) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	documentID, err := parseUUIDParam(r, "documentId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_document_id", "Invalid document ID")
		return
	}

	if err := h.Service.DeleteDocument(r.Context(), documentID, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"deleted": true}})
}

// ---------------------------------------------------------------------------
// Verifier Handlers
// ---------------------------------------------------------------------------

// ListVerifiers handles GET /api/v1/professional-profiles/{id}/verifiers.
func (h *Handlers) ListVerifiers(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	profileID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	verifiers, err := h.Service.ListVerifiers(r.Context(), profileID, tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": verifiers})
}

// CreateVerifier handles POST /api/v1/professional-profiles/{id}/verifiers.
func (h *Handlers) CreateVerifier(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	profileID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid profile ID")
		return
	}

	var input struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.UserID == "" {
		writeError(w, http.StatusBadRequest, "missing_user_id", "user_id is required")
		return
	}

	var verifierUserID pgtype.UUID
	if err := verifierUserID.Scan(input.UserID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id")
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
	writeJSON(w, http.StatusCreated, map[string]any{"data": verifier})
}
