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
// Approves the most recent pending ownership claim for the organization on
// behalf of the admin (audit A20 — admin "claim queue" action).
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

	var note string
	if r.Body != nil {
		var body struct {
			Note string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
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
// Returns all pending ownership claims with claimant and org details.
func (h *Handlers) ListPendingOwnershipClaims(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	claims, err := h.Queries.GetPendingOwnershipClaims(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to fetch pending claims")
		return
	}

	type claimResponse struct {
		ID               string  `json:"id"`
		OrganizationID   string  `json:"organization_id"`
		ClaimantUserID   string  `json:"claimant_user_id"`
		ClaimantUsername string  `json:"claimant_username"`
		OrgName          string  `json:"org_name"`
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
			OrgName:          c.OrgName,
			EvidenceURL:      c.EvidenceUrl,
			Notes:            c.Notes,
			CreatedAt:        c.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}
