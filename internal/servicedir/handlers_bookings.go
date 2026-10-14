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
//	POST  /api/v1/me/service-bookings/:id/complete — booker complete (user-provider rows)
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
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// writeMyBookingError is the /me/service-bookings object-route variant of
// writeServiceError (§111 hardening): a foreign-but-existing booking is
// indistinguishable from a missing one — ErrNotOwner and pgx.ErrNoRows
// both answer a uniform 404 booking_not_found so the user-facing surface
// never acts as an existence oracle. Org and admin routes keep the
// 403 not_owner distinction (privileged viewers may learn the row exists).
func writeMyBookingError(w http.ResponseWriter, err error, code, msg string) {
	if errors.Is(err, ErrNotOwner) || errors.Is(err, pgx.ErrNoRows) {
		errpkg.WriteError(w, http.StatusNotFound, "booking_not_found", "Booking not found")
		return
	}

	writeServiceError(w, err, code, msg)
}

// packagesAvailable gates every booking route on service_packages.enabled
// (404 when off — disabled features are invisible per convention).
func (h *Handlers) packagesAvailable(w http.ResponseWriter, r *http.Request) bool {
	if h.service == nil || !h.service.PackagesEnabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "service_packages_disabled", "Service packages are not available")
		return false
	}

	return true
}

// bookingSurfaceAvailable gates the /me/service-bookings read/cancel
// routes — they serve both package and mentorship bookings, so either
// flag being on keeps them live (000824). Booking-kind specifics are
// enforced inside the service layer.
func (h *Handlers) bookingSurfaceAvailable(w http.ResponseWriter, r *http.Request) bool {
	packages := h.service != nil && h.service.PackagesEnabled(r.Context())
	mentorship := h.service != nil && h.service.MentorshipEnabled(r.Context())
	if !packages && !mentorship {
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

	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// The feature gate follows the listing's kind — mentorship-kind
	// listings (000824) are gated on mentorship.enabled, not
	// service_packages.enabled. The listing lookup doubles as the
	// existence check the service layer would otherwise do.
	listing, err := h.service.GetListingByContentID(r.Context(), contentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}
		slog.Error("servicedir: booking listing lookup failed", "content_id", contentID, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "booking_failed", "Failed to create booking")
		return
	}

	if listing.ListingKind == ListingKindMentorship {
		if h.service == nil || !h.service.MentorshipEnabled(r.Context()) {
			errpkg.WriteError(w, http.StatusNotFound, "mentorship_disabled", "Mentorship is not available")
			return
		}

		// §111: mentorship-kind bookings additionally require
		// mentorship.request — the route-level service.booking.create
		// grant (member+) covers package/service bookings but must not
		// let a business-only member mint mentor requests. Conditional
		// (not a second route gate): the kind isn't known until the
		// listing loads.
		if h.rbac == nil || !h.rbac.HasPermission(r.Context(), user.UserID, "mentorship.request") {
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to perform this action")
			return
		}
	} else if !h.packagesAvailable(w, r) {
		return
	}

	var input CreateBookingInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// M5: return_url becomes the payment provider's success_url — an
	// open-redirect vector unless it stays same-origin. Rejected before
	// any booking row is written.
	if !validReturnURL(input.ReturnURL, r.Host) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_return_url", "return_url must be a site-relative path or a same-origin URL")
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

// HandlePricePreview handles GET /services/:id/price-preview — the live
// quote behind the booking modal's Apply button (?tier_id=&coupon_code=).
// No mutation. Read-only, so guests may preview too (M4): the route uses
// OptionalAuth and per-user coupon caps (per_user_limit, first_time_only)
// only apply when a viewer is present — anonymous previews quote the base
// numbers without consuming or validating personal caps. Coupon rejections
// return 200 with coupon_valid:false + error_code so the UI can show the
// undiscounted quote beside the failure reason; an unknown/inactive tier
// is a hard 400 invalid_tier.
func (h *Handlers) HandlePricePreview(w http.ResponseWriter, r *http.Request) {
	// Optional viewer — nil for anonymous previews (M4).
	user := auth.FromContext(r.Context())
	var bookerID pgtype.UUID
	if user != nil {
		bookerID = user.UserID
	}

	if !h.packagesAvailable(w, r) {
		return
	}

	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	tierID := r.URL.Query().Get("tier_id")
	couponCode := r.URL.Query().Get("coupon_code")
	if couponCode == "" {
		// `coupon` is the spec-named alias for the same param.
		couponCode = r.URL.Query().Get("coupon")
	}

	toDTO := func(q Quote, couponValid bool, errCode string) PricePreviewDTO {
		return PricePreviewDTO{
			Base:        q.Base,
			Discount:    q.Discount,
			Total:       q.Total,
			Fee:         q.Fee,
			BalanceDue:  q.Balance,
			Currency:    q.Currency,
			CouponValid: couponValid,
			ErrorCode:   errCode,
		}
	}

	quote, err := h.service.PricePreview(r.Context(), bookerID, contentID, tierID, couponCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}

		if isCouponQuoteError(err) {
			// Re-quote without the coupon so the base numbers still render;
			// the tier was already validated (it resolves before the coupon
			// chain), so a second failure is a genuine error.
			baseQuote, berr := h.service.PricePreview(r.Context(), bookerID, contentID, tierID, "")
			if berr != nil {
				if errors.Is(berr, pgx.ErrNoRows) {
					errpkg.WriteError(w, http.StatusNotFound, "service_not_found", "Service not found")
					return
				}

				slog.Error("servicedir: price preview base quote failed", "content_id", contentID, "error", berr)
				writeServiceError(w, berr, "price_preview_failed", "Failed to compute price preview")

				return
			}

			errpkg.WriteJSON(w, http.StatusOK, map[string]any{
				"data": toDTO(baseQuote, false, couponQuoteErrorCode(err)),
			})

			return
		}

		slog.Error("servicedir: price preview failed", "content_id", contentID, "error", err)
		writeServiceError(w, err, "price_preview_failed", "Failed to compute price preview")

		return
	}

	// coupon_valid is only meaningful when a coupon code was supplied —
	// a bare quote reports false rather than implying a check ran.
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toDTO(quote, strings.TrimSpace(couponCode) != "", "")})
}

// HandleMyBookings handles GET /me/service-bookings — caller's booking
// history. role=booker (default) lists bookings the caller made;
// role=provider (000824) lists bookings the caller provides — mentorship
// requests today. ?status=&intent=&kind=&page=&limit= filter both roles.
func (h *Handlers) HandleMyBookings(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	role := r.URL.Query().Get("role")
	if role == "" {
		role = "booker"
	}
	kind := r.URL.Query().Get("kind")

	packages := h.service != nil && h.service.PackagesEnabled(r.Context())
	mentorship := h.service != nil && h.service.MentorshipEnabled(r.Context())

	var bookings []BookingDTO
	var total int64
	var err error

	switch role {
	case "booker":
		// Mentorship bookings live here even when packages are off — the
		// endpoint serves either flag.
		if !packages && !mentorship {
			errpkg.WriteError(w, http.StatusNotFound, "service_packages_disabled", "Service packages are not available")
			return
		}
		if kind == ListingKindMentorship && !mentorship {
			errpkg.WriteError(w, http.StatusNotFound, "mentorship_disabled", "Mentorship is not available")
			return
		}

		page, limit := parsePageParams(r)
		bookings, total, err = h.service.ListMyBookings(r.Context(), user.UserID,
			r.URL.Query().Get("status"), r.URL.Query().Get("intent"), kind, page, limit)
	case "provider":
		// Provider-side inbox is mentorship-gated (000824).
		if !mentorship {
			errpkg.WriteError(w, http.StatusNotFound, "mentorship_disabled", "Mentorship is not available")
			return
		}

		page, limit := parsePageParams(r)
		bookings, total, err = h.service.ListProviderBookings(r.Context(), user.UserID,
			r.URL.Query().Get("status"), r.URL.Query().Get("intent"), kind, page, limit)
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_role", "role must be booker|provider")
		return
	}

	if err != nil {
		slog.Error("servicedir: my bookings failed", "role", role, "error", err)
		writeServiceError(w, err, "list_failed", "Failed to list bookings")

		return
	}

	page, limit := parsePageParams(r)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": bookings,
		"meta": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// HandleMyBookingRespond handles POST /me/service-bookings/:id/respond —
// the provider's answer on a user-provider (mentorship) booking
// (000824). Route carries mentorship.respond. Body:
// {"action":"accept|decline|complete|cancel","note":"optional"}.
func (h *Handlers) HandleMyBookingRespond(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.MentorshipEnabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "mentorship_disabled", "Mentorship is not available")
		return
	}

	bookingID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var input BookingRespondInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Action == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_action", "action is required")
		return
	}

	booking, err := h.service.RespondToBooking(r.Context(), user.UserID, bookingID, input.Action, input.Note)
	if err != nil {
		if !errors.Is(err, ErrNotOwner) && !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("servicedir: booking respond failed", "booking_id", bookingID, "action", input.Action, "error", err)
		}
		writeMyBookingError(w, err, "respond_failed", "Failed to respond to booking")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": booking})
}

// HandleMyBookingDetail handles GET /me/service-bookings/:id.
func (h *Handlers) HandleMyBookingDetail(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.bookingSurfaceAvailable(w, r) {
		return
	}

	bookingID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	booking, err := h.service.GetBooking(r.Context(), bookingID, user.UserID, false)
	if err != nil {
		if !errors.Is(err, ErrNotOwner) && !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("servicedir: get booking failed", "booking_id", bookingID, "error", err)
		}
		writeMyBookingError(w, err, "get_failed", "Failed to get booking")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": booking})
}

// HandleMyBookingPay handles POST /me/service-bookings/:id/pay — resumes a
// pending fee checkout on a paid booking (§111). Returns a fresh
// checkout_url the client redirects to. Route carries
// service.booking.create + the idempotency middleware (Rule 9).
func (h *Handlers) HandleMyBookingPay(w http.ResponseWriter, r *http.Request) {
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

	var input BookingPayInput
	_ = json.NewDecoder(r.Body).Decode(&input) // email/return_url optional for non-Paystack providers

	// M5: same same-origin gate as the create path — the resume mints a
	// fresh provider session carrying return_url as its success_url.
	if !validReturnURL(input.ReturnURL, r.Host) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_return_url", "return_url must be a site-relative path or a same-origin URL")
		return
	}

	payment, err := h.service.ResumeBookingPayment(r.Context(), bookingID, user.UserID, input)
	if err != nil {
		if !errors.Is(err, ErrNotOwner) && !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("servicedir: resume booking payment failed", "booking_id", bookingID, "error", err)
		}
		writeMyBookingError(w, err, "payment_resume_failed", "Failed to resume booking payment")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": payment})
}

// HandleMyBookingCancel handles POST /me/service-bookings/:id/cancel.
func (h *Handlers) HandleMyBookingCancel(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.bookingSurfaceAvailable(w, r) {
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
		if !errors.Is(err, ErrNotOwner) && !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("servicedir: cancel booking failed", "booking_id", bookingID, "error", err)
		}
		writeMyBookingError(w, err, "cancel_failed", "Failed to cancel booking")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": booking})
}

// HandleMyBookingComplete handles POST /me/service-bookings/:id/complete —
// the booker arm of "either party completes" (000824): confirmed →
// fulfilled on user-provider (mentorship) rows only. Replaces the mentee
// arm of the legacy PATCH /mentorship/:id/complete route.
func (h *Handlers) HandleMyBookingComplete(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.bookingSurfaceAvailable(w, r) {
		return
	}

	bookingID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	booking, err := h.service.CompleteMyBooking(r.Context(), bookingID, user.UserID)
	if err != nil {
		if !errors.Is(err, ErrNotOwner) && !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("servicedir: complete booking failed", "booking_id", bookingID, "error", err)
		}
		writeMyBookingError(w, err, "complete_failed", "Failed to complete booking")

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
		if !errors.Is(err, ErrNotOwner) && !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("servicedir: dispute booking failed", "booking_id", bookingID, "error", err)
		}
		writeMyBookingError(w, err, "dispute_failed", "Failed to dispute booking")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": booking})
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
// (?status=&intent=&org_id=&kind=&provider_id=&booker_id=). Requires
// service.booking.admin (route).
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
		r.URL.Query().Get("org_id"), r.URL.Query().Get("kind"),
		r.URL.Query().Get("provider_id"), r.URL.Query().Get("booker_id"),
		page, limit)
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
