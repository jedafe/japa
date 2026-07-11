package admin

import (
	"encoding/json"
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

	writeJSON(w, http.StatusOK, map[string]any{"data": alerts})
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
	var claimID pgtype.UUID
	err = h.DB.Reader.QueryRow(ctx,
		`SELECT id FROM ownership_claims
		 WHERE organization_id = $1 AND tenant_id = $2 AND status = 'pending'
		 ORDER BY created_at DESC LIMIT 1`,
		orgID, tenantID,
	).Scan(&claimID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no_pending_claim", "No pending ownership claim for this organization")
		return
	}

	if _, err := h.DB.Writer.Exec(ctx,
		`UPDATE ownership_claims
		 SET status = 'approved', reviewed_by = $1, resolution_note = $2, reviewed_at = now()
		 WHERE id = $3 AND tenant_id = $4`,
		user.UserID, note, claimID, tenantID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to approve ownership claim")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"claim_id": claimID.String(), "status": "approved"}})
}
