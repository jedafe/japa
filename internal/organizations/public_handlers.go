package organizations

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jackc/pgx/v5/pgtype"
)

// GetPublicOrganization handles GET /organizations/{slug} (public). Returns the
// lean org header for ACTIVE orgs; non-active/deleted/missing → 404 (the query
// returns no row → handleServiceError maps pgx.ErrNoRows to 404).
//
// The {slug} slot also accepts a UUID: when the param parses as a UUID and the
// caller is an authenticated member of that org, the full gen.Organization row
// is returned (for the org admin dashboard). This avoids a routing conflict
// between {slug} and {id} on the same path.
func (h *Handlers) GetPublicOrganization(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	param := chi.URLParam(r, "slug")

	// Detect UUID: if it parses as a UUID, treat it as an ID lookup.
	var asUUID pgtype.UUID
	if err := asUUID.Scan(param); err == nil && asUUID.Valid {
		// Authed member path: return the full row for the admin dashboard.
		if user := auth.FromContext(r.Context()); user != nil {
			org, err := h.service.GetOrganization(r.Context(), asUUID, tenantID)
			if err != nil {
				handleServiceError(w, err)
				return
			}
			isMember, err := h.service.IsMember(r.Context(), org.ID, user.UserID)
			if err != nil || !isMember {
				writeError(w, http.StatusForbidden, "forbidden", "You are not a member of this organization")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": org})
			return
		}
		// Guest requesting by UUID: fall through to 404 (guests must use the
		// public slug path, which returns the lean DTO for active orgs only).
		writeError(w, http.StatusNotFound, "not_found", "Organization not found")
		return
	}

	org, err := h.service.GetPublicOrganizationBySlug(r.Context(), param, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": toPublicOrg(org)})
}

// ListPublicMembers handles GET /organizations/{id}/members (public). The
// active-org gate (requireActiveOrgByID) has already 404'd non-active orgs.
func (h *Handlers) ListPublicMembers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}
	orgID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	rows, err := h.service.ListPublicMembers(r.Context(), orgID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toOrgMembers(rows)})
}

// ListPublicOrganizations handles GET /organizations (public): active orgs,
// paginated with search and filters. Query params: q (search by name/slug),
// org_type (filter: business/ngo/government/embassy/university/media/community),
// verified (filter: "true"=verified only [default], "false"=unverified only,
// empty=all), page_size, page_offset.
func (h *Handlers) ListPublicOrganizations(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	orgType := strings.TrimSpace(r.URL.Query().Get("org_type"))
	verified := strings.TrimSpace(r.URL.Query().Get("verified"))
	if verified == "" {
		// Default: only verified orgs (preserves original behavior).
		verified = "true"
	}

	rows, total, err := h.service.ListPublicOrganizations(r.Context(), tenantID, search, orgType, verified, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	totalPages := int64(0)
	if limit > 0 {
		totalPages = total / int64(limit)
		if total%int64(limit) > 0 {
			totalPages++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": toPublicOrgList(rows),
		"meta": map[string]any{
			"total":       total,
			"page":        offset / limit,
			"total_pages": totalPages,
		},
	})
}
