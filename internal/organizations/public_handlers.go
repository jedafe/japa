package organizations

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// GetPublicOrganization handles GET /organizations/{slug} (public). Returns the
// lean org header for ACTIVE orgs; non-active/deleted/missing → 404 (the query
// returns no row → handleServiceError maps pgx.ErrNoRows to 404).
// // The {slug} slot also accepts a UUID: when the param parses as a UUID and the
// caller is an authenticated member of that org, the full gen.Organization row
// is returned (for the org admin dashboard). This avoids a routing conflict
// between {slug} and {id} on the same path.
func (h *Handlers) GetPublicOrganization(w http.ResponseWriter, r *http.Request) {
	param := chi.URLParam(r, "slug")

	// Detect UUID: if it parses as a UUID, treat it as an ID lookup.
	var asUUID pgtype.UUID
	if err := asUUID.Scan(param); err == nil && asUUID.Valid {
		// Authed member path: return the full row for the admin dashboard.
		if user := auth.FromContext(r.Context()); user != nil {
			org, err := h.service.GetOrganization(r.Context(), asUUID)
			if err != nil {
				handleServiceError(w, err)
				return
			}

			isMember, err := h.service.IsMember(r.Context(), org.ID, user.UserID)
			if err != nil || !isMember {
				errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You are not a member of this organization")
				return
			}

			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": org})

			return
		}
		// Guest requesting by UUID: fall through to 404 (guests must use the
		// public slug path, which returns the lean DTO for active orgs only).
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Organization not found")

		return
	}

	org, err := h.service.GetPublicOrganizationBySlug(r.Context(), param)
	if err != nil {
		// 001045: seeded bank "orgs" moved to informational bank entities — a
		// slug that misses organizations but resolves in banks gets a typed
		// code so the frontend can 301 to the wiki bank page.
		if errors.Is(err, pgx.ErrNoRows) {
			if relocated, berr := h.service.BankExistsBySlug(r.Context(), param); berr == nil && relocated {
				errpkg.WriteError(w, http.StatusNotFound, "org_relocated", "This listing moved to the wiki bank directory")
				return
			}
		}
		handleServiceError(w, err)
		return
	}

	// Build the public org DTO, then resolve the owner's public profile for
	// the "Org Leader" sidebar widget. Best-effort: a missing/deleted owner
	// degrades to a nil owner field (the widget shows its empty state).
	orgDTO := toPublicOrg(org)
	orgDTO.LogoURL = h.resolveLogoURL(r.Context(), org.LogoMediaID)
	orgDTO.HeaderURL = h.resolveHeaderURL(r.Context(), org.HeaderMediaID)
	h.resolveLiterals(r, &orgDTO)

	if ownerRow, oErr := h.service.GetOrgOwnerPublicProfile(r.Context(), org.ID); oErr == nil {
		orgDTO.Owner = &OrgOwnerResponse{
			ID:          ownerRow.ID,
			Username:    ownerRow.Username,
			DisplayName: ownerRow.DisplayName,
			AvatarURL:   ownerRow.AvatarUrl,
		}
	}

	// JSON-LD structured data (Section 14 / Section 26). Resolve the
	// admin-managed Organization template, interpolating the org's field
	// values. Best-effort: a missing template or resolution error degrades
	// to no json_ld field. Emitted as a sibling of `data`.
	resp := map[string]any{"data": orgDTO}

	if h.jsonldResolver != nil {
		if jsonLd, jErr := h.jsonldResolver.ResolveJSONLD(r.Context(), "Organization", h.orgJSONLDFields(r.Context(), org)); jErr == nil && len(jsonLd) > 0 {
			resp["json_ld"] = json.RawMessage(jsonLd)
		} else if jErr != nil {
			slog.Warn("organizations: JSON-LD template resolution failed", "slug", param, "error", jErr)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, resp)
}

// ListPublicMembers handles GET /organizations/{id}/members (public). The
// active-org gate (requireActiveOrgByID) has already 404'd non-active orgs.
func (h *Handlers) ListPublicMembers(w http.ResponseWriter, r *http.Request) {
	orgID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	rows, err := h.service.ListPublicMembers(r.Context(), orgID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	members := toOrgMembers(rows)
	h.resolveLiterals(r, &members)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": members})
}

// ListPublicOrganizations handles GET /organizations (public): active orgs,
// paginated with search and filters. Query params: q (search by name/slug),
// org_type (filter: business/ngo/government/embassy/university/media/community),
// verified (trust filter — "claimed"=orgs with an owner [default],
// "verified"/"true"=badge holders, "unverified"/"false"=no badge,
// "all"=all active orgs), page_size, page_offset.
func (h *Handlers) ListPublicOrganizations(w http.ResponseWriter, r *http.Request) {
	limit := parsePageSize(r)
	offset := parsePageOffset(r)
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	orgType := strings.TrimSpace(r.URL.Query().Get("org_type"))

	// The directory default is claimed orgs — organizations with a real
	// operator behind the page. Legacy "true"/"false" values keep working.
	filter := strings.TrimSpace(r.URL.Query().Get("verified"))
	switch filter {
	case "", "claimed":
		filter = "claimed"
	case "true", "verified":
		filter = "verified"
	case "false", "unverified":
		filter = "unverified"
	case "all":
		// The SQL treats "" as "no filter".
		filter = ""
	}

	rows, total, err := h.service.ListPublicOrganizations(r.Context(), search, orgType, filter, limit, offset)
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

	orgs := toPublicOrgList(rows)
	for i := range orgs {
		orgs[i].LogoURL = h.resolveLogoURL(r.Context(), orgs[i].LogoMediaID)
		orgs[i].HeaderURL = h.resolveHeaderURL(r.Context(), orgs[i].HeaderMediaID)
	}
	h.resolveLiterals(r, &orgs)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": orgs,
		"meta": map[string]any{
			"total":       total,
			"page":        offset / limit,
			"total_pages": totalPages,
		},
	})
}
