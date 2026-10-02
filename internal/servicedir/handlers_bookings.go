// Package servicedir — handlers_bookings.go provides the §111 booking /
// inquiry pipeline endpoints. Every route here is gated on
// service_packages.enabled (flag off → 404).
//
// Routes:
//
//	POST  /api/v1/services/:id/bookings            — inquiry or paid booking
//	GET   /api/v1/me/service-bookings              — caller's bookings+inquiries
//	GET   /api/v1/me/service-bookings/:id          — caller's booking detail
//	POST  /api/v1/me/service-bookings/:id/cancel   — booker cancel
//	GET   /api/v1/org/:org_id/packages             — org's package listings
//	GET   /api/v1/org/:org_id/service-bookings     — org bookings inbox
//	PATCH /api/v1/org/:org_id/service-bookings/:id — org status transition
//	GET   /api/v1/admin/service-bookings           — admin bookings view
//	PATCH /api/v1/admin/service-bookings/:id       — admin dispute resolution
//
// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
//
//	japaconnect-engineering/adr/ADR-123-service-packages.md
package servicedir

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// packagesAvailable gates every booking route on service_packages.enabled
// (404 when off — disabled features are invisible per convention).
func (h *Handlers) packagesAvailable(w http.ResponseWriter, r *http.Request) bool {
	if h.service == nil || !h.service.PackagesEnabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "service_packages_disabled", "Service packages are not available")
		return false
	}

	return true
}

// HandleCreateBooking handles POST /services/:id/bookings. Auth required
// (service.booking.create at route level); the payment-changing path runs
// behind mw.IdempotencyMiddleware (Rule 9) so the Idempotency-Key header
// propagates to the payments.idempotency_key column via ctx.
func (h *Handlers) HandleCreateBooking(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.packagesAvailable(w, r) {
		return
	}

	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var input CreateBookingInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	result, err := h.service.CreateBooking(r.Context(), user.UserID, contentID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}

		slog.Error("servicedir: create booking failed", "content_id", contentID, "error", err)
		writeServiceError(w, err, "booking_failed", "Failed to create booking")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// HandleMyBookings handles GET /me/service-bookings — caller's booking
// history (?status=&intent=&page=&limit=).
func (h *Handlers) HandleMyBookings(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.packagesAvailable(w, r) {
		return
	}

	page, limit := parsePageParams(r)

	bookings, total, err := h.service.ListMyBookings(r.Context(), user.UserID,
		r.URL.Query().Get("status"), r.URL.Query().Get("intent"), page, limit)
	if err != nil {
		slog.Error("servicedir: my bookings failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list bookings")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": bookings,
		"meta": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// HandleMyBookingDetail handles GET /me/service-bookings/:id.
func (h *Handlers) HandleMyBookingDetail(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.packagesAvailable(w, r) {
		return
	}

	bookingID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	booking, err := h.service.GetBooking(r.Context(), bookingID, user.UserID, false)
	if err != nil {
		slog.Error("servicedir: get booking failed", "booking_id", bookingID, "error", err)
		writeServiceError(w, err, "get_failed", "Failed to get booking")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": booking})
}

// HandleMyBookingCancel handles POST /me/service-bookings/:id/cancel.
func (h *Handlers) HandleMyBookingCancel(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.packagesAvailable(w, r) {
		return
	}

	bookingID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // reason is optional

	booking, err := h.service.CancelMyBooking(r.Context(), bookingID, user.UserID, body.Reason)
	if err != nil {
		slog.Error("servicedir: cancel booking failed", "booking_id", bookingID, "error", err)
		writeServiceError(w, err, "cancel_failed", "Failed to cancel booking")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": booking})
}

// HandleMyBookingDispute handles POST /me/service-bookings/:id/dispute —
// the booker reports a problem on a confirmed|fulfilled booking. The row
// lands in 'disputed' for admin resolution (§111).
func (h *Handlers) HandleMyBookingDispute(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.packagesAvailable(w, r) {
		return
	}

	bookingID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // reason is optional but recommended

	booking, err := h.service.DisputeMyBooking(r.Context(), bookingID, user.UserID, body.Reason)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "booking_not_found", "Booking not found")
			return
		}

		slog.Error("servicedir: dispute booking failed", "booking_id", bookingID, "error", err)
		writeServiceError(w, err, "dispute_failed", "Failed to dispute booking")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": booking})
}

// HandleOrgPackages handles GET /org/:org_id/packages — the org's package
// listings (public when the org is public).
func (h *Handlers) HandleOrgPackages(w http.ResponseWriter, r *http.Request) {
	if !h.packagesAvailable(w, r) {
		return
	}

	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}

	page, limit := parsePageParams(r)

	// Optional identity (OptionalAuth wraps /api/v1): org members see
	// drafts/in_review rows too; everyone else sees published only.
	var viewerID pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		viewerID = u.UserID
	}

	services, total, err := h.service.ListOrgPackages(r.Context(), orgID, viewerID, page, limit)
	if err != nil {
		slog.Error("servicedir: org packages failed", "org_id", orgID, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list packages")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": services,
		"meta": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// HandleOrgBookings handles GET /org/:org_id/service-bookings — the org's
// bookings inbox (?status=&intent=&listing_id=). Requires
// service.booking.manage (route) + org membership (service layer).
func (h *Handlers) HandleOrgBookings(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.packagesAvailable(w, r) {
		return
	}

	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}

	page, limit := parsePageParams(r)

	bookings, total, err := h.service.ListOrgBookings(r.Context(), orgID, user.UserID,
		r.URL.Query().Get("status"), r.URL.Query().Get("intent"),
		r.URL.Query().Get("listing_id"), page, limit)
	if err != nil {
		slog.Error("servicedir: org bookings failed", "org_id", orgID, "error", err)
		writeServiceError(w, err, "list_failed", "Failed to list bookings")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": bookings,
		"meta": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// HandleOrgBookingTransition handles PATCH
// /org/:org_id/service-bookings/:id — org status transition
// (confirmed|declined|fulfilled|cancelled).
func (h *Handlers) HandleOrgBookingTransition(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.packagesAvailable(w, r) {
		return
	}

	orgID, ok := scanUUIDParam(w, r, "org_id")
	if !ok {
		return
	}

	bookingID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var input OrgBookingActionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Status == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_status", "status is required")
		return
	}

	booking, err := h.service.OrgTransition(r.Context(), orgID, bookingID, user.UserID, input)
	if err != nil {
		slog.Error("servicedir: org transition failed", "booking_id", bookingID, "status", input.Status, "error", err)
		writeServiceError(w, err, "transition_failed", "Failed to update booking")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": booking})
}

// HandleAdminBookings handles GET /admin/service-bookings — all bookings
// (?status=&intent=&org_id=). Requires service.booking.admin (route).
func (h *Handlers) HandleAdminBookings(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if !h.packagesAvailable(w, r) {
		return
	}

	page, limit := parsePageParams(r)

	bookings, total, err := h.service.AdminListBookings(r.Context(),
		r.URL.Query().Get("status"), r.URL.Query().Get("intent"),
		r.URL.Query().Get("org_id"), page, limit)
	if err != nil {
		slog.Error("servicedir: admin bookings failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list bookings")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": bookings,
		"meta": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// HandleAdminBookingTransition handles PATCH /admin/service-bookings/:id —
// dispute resolution / forced status transition
// (service.booking.admin at route level).
func (h *Handlers) HandleAdminBookingTransition(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if !h.packagesAvailable(w, r) {
		return
	}

	bookingID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var input AdminBookingActionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Status == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_status", "status is required")
		return
	}

	booking, err := h.service.AdminSetBookingStatus(r.Context(), bookingID, input)
	if err != nil {
		slog.Error("servicedir: admin booking transition failed", "booking_id", bookingID, "error", err)
		writeServiceError(w, err, "transition_failed", "Failed to update booking")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": booking})
}

// AdminGetBooking exposes a single booking for the admin view
// (GET /admin/service-bookings/:id).
func (h *Handlers) AdminGetBooking(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.packagesAvailable(w, r) {
		return
	}

	bookingID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	booking, err := h.service.GetBooking(r.Context(), bookingID, user.UserID, true)
	if err != nil {
		slog.Error("servicedir: admin get booking failed", "booking_id", bookingID, "error", err)
		writeServiceError(w, err, "get_failed", "Failed to get booking")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": booking})
}
