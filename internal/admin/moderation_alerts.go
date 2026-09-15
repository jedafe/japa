package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ListModerationAlerts handles GET /api/v1/admin/moderation/alerts.
// Supports an optional ?status= filter (open|acknowledged|resolved).
func (h *Handlers) ListModerationAlerts(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}


	status := r.URL.Query().Get("status")

	alerts, err := gen.New(h.DB.Reader).ListModerationAlerts(r.Context(),

		status,
	)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to load moderation alerts")
		return
	}

	// Count total matching alerts for pagination metadata.
	var total int32

	countErr := h.DB.Reader.QueryRow(r.Context(), `SELECT COUNT(*)::int FROM moderation_alerts
		 WHERE (NULLIF($1, '') IS NULL OR status = $1)`, status).Scan(&total)
	if countErr != nil {
		slog.Warn("moderation: counting alerts", "error", countErr)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": alerts,
		"meta": map[string]any{"total": total},
	})
}

// AcknowledgeModerationAlert handles POST /api/v1/admin/moderation/alerts/{id}/acknowledge.
func (h *Handlers) AcknowledgeModerationAlert(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}


	id, err := scanUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid alert ID")
		return
	}

	alert, err := gen.New(h.DB.Writer).AcknowledgeModerationAlert(r.Context(), gen.AcknowledgeModerationAlertParams{
		ID: id,

		AcknowledgedBy: user.UserID,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to acknowledge alert")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": alert})
}

// ResolveModerationAlert handles POST /api/v1/admin/moderation/alerts/{id}/resolve.
func (h *Handlers) ResolveModerationAlert(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}


	id, err := scanUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid alert ID")
		return
	}

	alert, err := gen.New(h.DB.Writer).ResolveModerationAlert(r.Context(), gen.ResolveModerationAlertParams{
		ID: id,

		ResolvedBy: user.UserID,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to resolve alert")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": alert})
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
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}


	orgID, err := scanUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
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

	claimID, err := h.Queries.GetPendingOwnershipClaim(ctx,
		orgID,
	)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "no_pending_claim", "No pending ownership claim for this organization")
		return
	}

	if action == "reject" {
		rejected := "rejected"
		if err := h.Queries.ReviewOwnershipClaim(ctx, gen.ReviewOwnershipClaimParams{
			ID: claimID,

			Status:         rejected,
			ReviewedBy:     user.UserID,
			ResolutionNote: &note,
		}); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to reject ownership claim")
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"claim_id": claimID.String(), "status": "rejected"}})

		return
	}

	if err := h.Queries.ApproveOwnershipClaim(ctx, gen.ApproveOwnershipClaimParams{
		ID: claimID,

		ReviewedBy:     user.UserID,
		ResolutionNote: &note,
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to approve ownership claim")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"claim_id": claimID.String(), "status": "approved"}})
}

// ListPendingOwnershipClaims handles GET /api/v1/admin/organizations/pending-claims.
// Returns ownership claims with claimant and org details. An optional ?status=
// query param filters by status (pending|approved|rejected|withdrawn); omitted
// or empty defaults to "pending" (the review queue view), and "all" returns
// every claim for history/audit. Ref: sections/07-organization-profiles-claim-ownership.md.
func (h *Handlers) ListPendingOwnershipClaims(w http.ResponseWriter, r *http.Request) {

	// Default to pending so existing callers (e.g. the /organisations page's
	// "Approve Claim" action map) keep receiving only actionable claims.
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}

	claims, err := h.Queries.ListOwnershipClaimsForAdmin(r.Context(), status)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "db_error", "Failed to fetch pending claims")
		return
	}

	type claimResponse struct {
		ID                 string  `json:"id"`
		OrganizationID     string  `json:"organization_id"`
		ClaimantUserID     string  `json:"claimant_user_id"`
		ClaimantUsername   string  `json:"claimant_username"`
		ClaimantEmail      string  `json:"claimant_email"`
		OrgName            string  `json:"org_name"`
		OrgSlug            string  `json:"org_slug"`
		Status             string  `json:"status"`
		EvidenceURL        *string `json:"evidence_url"`
		Notes              *string `json:"notes"`
		FormSubmissionID   *string `json:"form_submission_id"`
		ResolutionNote     *string `json:"resolution_note"`
		ReviewedByUsername *string `json:"reviewed_by_username"`
		ReviewedAt         *string `json:"reviewed_at"`
		CreatedAt          string  `json:"created_at"`
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

		if c.FormSubmissionID.Valid {
			s := uuid.UUID(c.FormSubmissionID.Bytes).String()
			items[i].FormSubmissionID = &s
		}

		if c.ResolutionNote != nil {
			items[i].ResolutionNote = c.ResolutionNote
		}

		if c.ReviewedByUsername != nil {
			items[i].ReviewedByUsername = c.ReviewedByUsername
		}

		if c.ReviewedAt.Valid {
			s := c.ReviewedAt.Time.Format("2006-01-02T15:04:05Z")
			items[i].ReviewedAt = &s
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetClaimFormSubmission handles GET /admin/organizations/claims/{id}/submission.
// Returns the form submission data linked to an ownership claim so the admin
// can inspect the claimant's submitted field values during claim review.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
func (h *Handlers) GetClaimFormSubmission(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}


	claimID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid claim ID")
		return
	}

	// Fetch the claim to get the form_submission_id.
	claim, err := h.Queries.GetOwnershipClaimByID(r.Context(),
		pgtype.UUID{Bytes: claimID, Valid: true},
	)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "claim_not_found", "Claim not found")
		return
	}

	if !claim.FormSubmissionID.Valid {
		errpkg.WriteError(w, http.StatusNotFound, "no_submission", "This claim has no linked form submission")
		return
	}

	submission, err := h.Queries.GetFormSubmissionByID(r.Context(),
		claim.FormSubmissionID,
	)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "submission_not_found", "Form submission not found")
		return
	}

	// Parse the field_values JSON for the response.
	var fieldValues any
	if len(submission.FieldValues) > 0 {
		_ = json.Unmarshal(submission.FieldValues, &fieldValues)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":           uuid.UUID(submission.ID.Bytes).String(),
			"form_id":      uuid.UUID(submission.FormID.Bytes).String(),
			"field_values": fieldValues,
			"status":       submission.Status,
			"created_at":   submission.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
			"claim_id":     claimID.String(),
		},
	})
}
