package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
)

// ListModerationAlerts handles GET /api/v1/admin/moderation/alerts.
// Supports an optional ?status= filter (open|acknowledged|resolved).
func (h *Handlers) ListModerationAlerts(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	alerts, err := gen.New(h.DB.Reader).ListModerationAlerts(r.Context(), gen.ListModerationAlertsParams{
		TenantID: tenantID,
		Status:   status,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to load moderation alerts")
		return
	}

	// Count total matching alerts for pagination metadata.
	var total int32
	countErr := h.DB.Reader.QueryRow(r.Context(),
		`SELECT COUNT(*)::int FROM moderation_alerts
		 WHERE tenant_id = $1 AND (NULLIF($2, '') IS NULL OR status = $2)`,
		tenantID, status,
	).Scan(&total)
	if countErr != nil {
		slog.Warn("moderation: counting alerts", "error", countErr)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": alerts,
		"meta": map[string]any{"total": total},
	})
}

// AcknowledgeModerationAlert handles POST /api/v1/admin/moderation/alerts/{id}/acknowledge.
func (h *Handlers) AcknowledgeModerationAlert(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := scanUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid alert ID")
		return
	}

	alert, err := gen.New(h.DB.Writer).AcknowledgeModerationAlert(r.Context(), gen.AcknowledgeModerationAlertParams{
		ID:           id,
		TenantID:     tenantID,
		AcknowledgedBy: user.UserID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to acknowledge alert")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": alert})
}

// ResolveModerationAlert handles POST /api/v1/admin/moderation/alerts/{id}/resolve.
func (h *Handlers) ResolveModerationAlert(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := scanUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid alert ID")
		return
	}

	alert, err := gen.New(h.DB.Writer).ResolveModerationAlert(r.Context(), gen.ResolveModerationAlertParams{
		ID:        id,
		TenantID:  tenantID,
		ResolvedBy: user.UserID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to resolve alert")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": alert})
}

// scanUUIDParam reads a URL parameter and scans it into a pgtype.UUID.
func scanUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return id, err
	}
	return id, nil
}

// ClaimOrganization handles POST /api/v1/admin/organizations/{id}/claim.
// Approves or rejects the most recent pending ownership claim for the
// organization on behalf of the admin (audit A20 — admin "claim queue" action).
// Body: { action: "approve" | "reject", note?: string }. When `action` is
// omitted it defaults to "approve" for backwards compatibility. Rejecting a
// claim sets its status to "rejected" via ReviewOwnershipClaim.
func (h *Handlers) ClaimOrganization(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	orgID, err := scanUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	var (
		note   string
		action = "approve"
	)
	if r.Body != nil {
		var body struct {
			Action string `json:"action"`
			Note   string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if body.Action == "approve" || body.Action == "reject" {
				action = body.Action
			}
			note = body.Note
		}
	}

	ctx := r.Context()
	claimID, err := h.Queries.GetPendingOwnershipClaim(ctx, gen.GetPendingOwnershipClaimParams{
		OrganizationID: orgID,
		TenantID:       tenantID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "no_pending_claim", "No pending ownership claim for this organization")
		return
	}

	if action == "reject" {
		rejected := "rejected"
		if err := h.Queries.ReviewOwnershipClaim(ctx, gen.ReviewOwnershipClaimParams{
			ID:             claimID,
			TenantID:       tenantID,
			Status:         rejected,
			ReviewedBy:     user.UserID,
			ResolutionNote: &note,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", "Failed to reject ownership claim")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"claim_id": claimID.String(), "status": "rejected"}})
		return
	}

	if err := h.Queries.ApproveOwnershipClaim(ctx, gen.ApproveOwnershipClaimParams{
		ID:             claimID,
		TenantID:       tenantID,
		ReviewedBy:     user.UserID,
		ResolutionNote: &note,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to approve ownership claim")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"claim_id": claimID.String(), "status": "approved"}})
}

// ListPendingOwnershipClaims handles GET /api/v1/admin/organizations/pending-claims.
// Returns ownership claims with claimant and org details. An optional ?status=
// query param filters by status (pending|approved|rejected|withdrawn); omitted
// or empty defaults to "pending" (the review queue view), and "all" returns
// every claim for history/audit. Ref: sections/07-organization-profiles-claim-ownership.md.
func (h *Handlers) ListPendingOwnershipClaims(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	// Default to pending so existing callers (e.g. the /organisations page's
	// "Approve Claim" action map) keep receiving only actionable claims.
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}

	claims, err := h.Queries.ListOwnershipClaimsForAdmin(r.Context(), gen.ListOwnershipClaimsForAdminParams{
		TenantID: tenantID,
		Column2:  status,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to fetch pending claims")
		return
	}

	type claimResponse struct {
		ID               string  `json:"id"`
		OrganizationID   string  `json:"organization_id"`
		ClaimantUserID   string  `json:"claimant_user_id"`
		ClaimantUsername string  `json:"claimant_username"`
		ClaimantEmail    string  `json:"claimant_email"`
		OrgName          string  `json:"org_name"`
		OrgSlug          string  `json:"org_slug"`
		Status           string  `json:"status"`
		EvidenceURL      *string `json:"evidence_url"`
		Notes            *string `json:"notes"`
		CreatedAt        string  `json:"created_at"`
	}

	items := make([]claimResponse, len(claims))
	for i, c := range claims {
		items[i] = claimResponse{
			ID:               c.ID.String(),
			OrganizationID:   c.OrganizationID.String(),
			ClaimantUserID:   c.ClaimantUserID.String(),
			ClaimantUsername: c.ClaimantUsername,
			ClaimantEmail:    c.ClaimantEmail,
			OrgName:          c.OrgName,
			OrgSlug:          c.OrgSlug,
			Status:           c.Status,
			EvidenceURL:      c.EvidenceUrl,
			Notes:            c.Notes,
			CreatedAt:        c.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}
