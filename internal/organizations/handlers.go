package organizations

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers provides HTTP handlers for organization management.
type Handlers struct {
	service *Service
}

// NewHandlers creates a Handlers instance with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("limit")
	if s == "" {
		s = r.URL.Query().Get("page_size")
	}
	if s == "" {
		return DefaultPageSize
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return DefaultPageSize
	}
	if n > int(MaxPageSize) {
		return MaxPageSize
	}
	return int32(n) //nolint:gosec // bounded by MaxPageSize check
}

func parsePageOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page")
	if s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return 0
		}
		return int32(n) //nolint:gosec // bounded by query parameter
	}
	s = r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return int32(n) //nolint:gosec // bounded by query parameter
}

func requireAuth(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

func requireTenantID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	tid := tenant.IDFromContext(r.Context())
	if !tid.Valid {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Tenant context required")
		return pgtype.UUID{}, false
	}
	return tid, true
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		slog.Error("service error", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

type createOrganizationRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Tagline     string `json:"tagline"`
	Description string `json:"description"`
	WebsiteURL  string `json:"website_url"`
	CountryCode string `json:"country_code"`
	OrgType     string `json:"org_type"`
}

// CreateOrganization handles POST /organizations.
func (h *Handlers) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	var req createOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	input := CreateOrganizationInput(req)

	result, err := h.service.CreateOrganization(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// GetOrganization handles GET /organizations/{slug}.
func (h *Handlers) GetOrganization(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")

	result, err := h.service.GetOrganizationBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// ListOrganizations handles GET /organizations.
func (h *Handlers) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	orgType := r.URL.Query().Get("org_type")
	countryCode := r.URL.Query().Get("country_code")
	search := r.URL.Query().Get("q")
	verified := r.URL.Query().Get("verified")
	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	items, total, err := h.service.ListOrganizations(r.Context(), tenantID, orgType, countryCode, search, verified, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	totalPages := int32(1)
	if limit > 0 && total > 0 {
		totalPages = int32((total + int64(limit) - 1) / int64(limit))
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items":       items,
			"total":       total,
			"total_pages": totalPages,
			"page":        offset / limit,
		},
	})
}

type updateOrganizationRequest struct {
	Name        *string `json:"name"`
	Tagline     *string `json:"tagline"`
	Description *string `json:"description"`
	WebsiteURL  *string `json:"website_url"`
	CountryCode *string `json:"country_code"`
	OrgType     *string `json:"org_type"`
}

// UpdateOrganization handles PATCH /organizations/{id}.
func (h *Handlers) UpdateOrganization(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	var req updateOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	input := UpdateOrganizationInput{}
	if req.Name != nil {
		input.Name = *req.Name
	}
	if req.Tagline != nil {
		input.Tagline = *req.Tagline
	}
	if req.Description != nil {
		input.Description = *req.Description
	}
	if req.WebsiteURL != nil {
		input.WebsiteURL = *req.WebsiteURL
	}
	if req.CountryCode != nil {
		input.CountryCode = *req.CountryCode
	}
	if req.OrgType != nil {
		input.OrgType = *req.OrgType
	}

	if err := h.service.UpdateOrganization(r.Context(), id, tenantID, user.UserID, input); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"message": "Organization updated"}})
}

// DeleteOrganization handles DELETE /organizations/{id}.
func (h *Handlers) DeleteOrganization(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	if err := h.service.DeleteOrganization(r.Context(), id, tenantID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"message": "Organization deleted"}})
}

// ListMembers handles GET /organizations/{id}/members.
func (h *Handlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	orgID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	result, err := h.service.ListMembers(r.Context(), orgID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

type addMemberRequest struct {
	UserID pgtype.UUID `json:"user_id"`
	Role   string      `json:"role"`
}

// AddMember handles POST /organizations/{id}/members.
func (h *Handlers) AddMember(w http.ResponseWriter, r *http.Request) {
	_, ok := requireAuth(w, r)
	if !ok {
		return
	}

	orgID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	var req addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	role := req.Role
	if role == "" {
		role = MemberRoleMember
	}

	if err := h.service.store.AddMember(r.Context(), gen.AddOrganizationMemberParams{
		OrganizationID: orgID,
		UserID:         req.UserID,
		Role:           role,
	}); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"message": "Member added"}})
}

// RemoveMember handles DELETE /organizations/{id}/members/{userId}.
func (h *Handlers) RemoveMember(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	orgID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	targetUserID, err := httpx.ParseUUIDParam(r, "userId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid member ID")
		return
	}

	if err := h.service.RemoveMember(r.Context(), orgID, user.UserID, targetUserID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"message": "Member removed"}})
}

type updateMemberRoleRequest struct {
	Role string `json:"role"`
}

// UpdateMemberRole handles PATCH /organizations/{id}/members/{userId}.
func (h *Handlers) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	orgID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	targetUserID, err := httpx.ParseUUIDParam(r, "userId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid member ID")
		return
	}

	var req updateMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	if err := h.service.UpdateMemberRole(r.Context(), orgID, user.UserID, targetUserID, req.Role); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"message": "Member role updated"}})
}

type inviteMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// InviteMember handles POST /organizations/{id}/invitations.
func (h *Handlers) InviteMember(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	orgID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	var req inviteMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	input := InviteMemberInput(req)

	result, err := h.service.InviteMember(r.Context(), orgID, tenantID, user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// ListInvitations handles GET /organizations/{id}/invitations.
func (h *Handlers) ListInvitations(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	orgID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	result, err := h.service.ListInvitations(r.Context(), orgID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// AcceptInvitation handles POST /organizations/invitations/{token}/accept.
func (h *Handlers) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	token := chi.URLParam(r, "token")
	tokenHash := hashToken(token)

	if err := h.service.AcceptInvitation(r.Context(), tokenHash, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"message": "Invitation accepted"}})
}

type claimOwnershipRequest struct {
	OrganizationID pgtype.UUID `json:"organization_id"`
	EvidenceURL    string      `json:"evidence_url"`
	Notes          string      `json:"notes"`
}

// ClaimOwnership handles POST /claim-ownership.
func (h *Handlers) ClaimOwnership(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	var req claimOwnershipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	input := ClaimOwnershipInput{
		EvidenceURL: req.EvidenceURL,
		Notes:       req.Notes,
	}

	result, err := h.service.ClaimOwnership(r.Context(), tenantID, user.UserID, req.OrganizationID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// ListClaims handles GET /claim-ownership.
func (h *Handlers) ListClaims(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")

	result, err := h.service.ListOwnershipClaims(r.Context(), tenantID, status, parsePageSize(r), parsePageOffset(r))
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

type reviewClaimRequest struct {
	Status         string `json:"status"`
	ResolutionNote string `json:"resolution_note"`
}

// ReviewClaim handles PATCH /claim-ownership/{id}.
func (h *Handlers) ReviewClaim(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	claimID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid claim ID")
		return
	}

	var req reviewClaimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	input := ReviewClaimInput(req)

	if err := h.service.ReviewOwnershipClaim(r.Context(), claimID, tenantID, user.UserID, input); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"message": "Claim reviewed"}})
}
