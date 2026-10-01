// service_bookings.go — §111 worker handlers for the service package
// booking pipeline:
//
//	service:booking_created        — notify org members of a new inquiry/booking
//	service:booking_status_changed — notify the booker of a status transition
//	service:booking_fee_settle     — recognize/refund the platform fee (critical)
//	service:booking_expire_pending — hourly SLA sweep on pending bookings (low)
//
// Worker handlers can't import internal/servicedir (it imports this
// package for task constants), so they work directly on the generated
// queries via dbPool. The §17 refund call goes through the injected
// ServiceBookingFeeRefunder seam — payments.Service implements it via an
// adapter in internal/payments (payments already imports worker).
//
// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
//
//	japaconnect-engineering/adr/ADR-123-service-packages.md
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// ServiceBookingFeeRefunder is the §17 seam the fee-settle task uses to
// refund the platform fee. Implemented by the payments-side adapter
// (payments.BookingFeeRefunder) — nil until RegisterServiceBookingFeeRefunder
// is called.
type ServiceBookingFeeRefunder interface {
	// RefundBookingFee refunds the platform-fee payment. Must be
	// idempotent — a second call for an already-refunded payment is a
	// no-op success.
	RefundBookingFee(ctx context.Context, paymentID pgtype.UUID, amount int64, reason string) error
}

var serviceBookingFeeRefunder ServiceBookingFeeRefunder

// RegisterServiceBookingFeeRefunder injects the §17 refund adapter.
func RegisterServiceBookingFeeRefunder(r ServiceBookingFeeRefunder) {
	serviceBookingFeeRefunder = r
}

// bookingNotificationType maps a booking status to the §35 notification
// type seeded in migration 000768.
func bookingNotificationType(status string) string {
	switch status {
	case "confirmed":
		return "service_booking_confirmed"
	case "declined":
		return "service_booking_declined"
	case "fulfilled":
		return "service_booking_fulfilled"
	case "cancelled", "refunded", "disputed":
		return "service_booking_cancelled"
	}

	return ""
}

// bookingPayload extracts the booking_id (+ optional status/action/
// payment_id) from a task payload. Permanent decode failures are
// skipped, not retried.
func bookingPayload(t *asynq.Task) (bookingID pgtype.UUID, status, action string, paymentID pgtype.UUID, err error) {
	var p struct {
		BookingID string `json:"booking_id"`
		Status    string `json:"status"`
		Action    string `json:"action"`
		PaymentID string `json:"payment_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return pgtype.UUID{}, "", "", pgtype.UUID{}, fmt.Errorf("decode payload: %w", err)
	}
	if p.BookingID == "" {
		return pgtype.UUID{}, "", "", pgtype.UUID{}, errors.New("missing booking_id")
	}

	if err := bookingID.Scan(p.BookingID); err != nil {
		return pgtype.UUID{}, "", "", pgtype.UUID{}, fmt.Errorf("invalid booking_id: %w", err)
	}

	if p.PaymentID != "" {
		if err := paymentID.Scan(p.PaymentID); err != nil {
			return pgtype.UUID{}, "", "", pgtype.UUID{}, fmt.Errorf("invalid payment_id: %w", err)
		}
	}

	return bookingID, p.Status, p.Action, paymentID, nil
}

// HandleServiceBookingCreated fans out a "new booking" notification to
// every member of the selling org (service_booking_new). Runs for inquiry
// bookings at creation time and for paid bookings only after the fee
// payment succeeds — the enqueue sites enforce that ordering.
func HandleServiceBookingCreated(ctx context.Context, t *asynq.Task) error {
	bookingID, _, _, paymentID, err := bookingPayload(t)
	if err != nil {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}

	q := gen.New(dbPool.Writer)

	bk, err := q.GetServiceBookingDetail(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: booking %s not found", asynq.SkipRetry, bookingID)
		}

		return fmt.Errorf("get booking: %w", err)
	}

	// Backfill payment_id when the intent-time attach raced or failed —
	// the org-inbox query joins payments on this FK, so a missing link
	// hides a paid booking forever.
	if paymentID.Valid && !bk.PaymentID.Valid {
		if _, err := q.BackfillServiceBookingPayment(ctx, gen.BackfillServiceBookingPaymentParams{
			ID: bookingID, PaymentID: paymentID,
		}); err != nil {
			slog.Error("task:service:booking_created: payment backfill failed",
				"booking_id", bookingID, "payment_id", paymentID, "error", err)
		} else {
			bk.PaymentID = paymentID
		}
	}

	switch bk.Status {
	case "pending", "confirmed":
		// pending: org hasn't responded yet — alert as new. confirmed is
		// only reachable via an admin force-transition while unpaid; the
		// booking just became actionable, so the org still needs the ping.
	case "disputed":
		return nil // dispute queue, not the inbox
	default:
		// declined/cancelled/refunded/fulfilled — the booking is closed but
		// this task only runs on payment *success*, so a captured fee on a
		// terminal booking must be refunded (late checkout past the SLA
		// sweep, booker-cancel during payment, etc.).
		if bk.Intent == "booking" && bk.PaymentID.Valid {
			payload, _ := json.Marshal(map[string]string{
				"booking_id": bk.ID.String(),
				"action":     "refund",
			})
			if err := taskEnqueuer.Enqueue(TypeServiceBookingFeeSettle, payload); err != nil {
				return fmt.Errorf("enqueue fee refund for terminal booking: %w", err)
			}
		}

		return nil
	}

	members, err := q.ListOrganizationMembers(ctx, bk.OrgID)
	if err != nil {
		return fmt.Errorf("list org members: %w", err)
	}

	actorType := "user"
	subjectType := "service_booking"
	title := fmt.Sprintf("New %s: %s", map[bool]string{true: "booking", false: "inquiry"}[bk.Intent == "booking"], bk.ListingTitle)
	body := fmt.Sprintf("A %s was submitted for %s. Respond before the SLA expires.", bk.Intent, bk.ListingTitle)

	orgRef := bk.OrgID.String()
	if bk.OrgSlug != nil && *bk.OrgSlug != "" {
		orgRef = *bk.OrgSlug
	}

	actionURL := fmt.Sprintf("/org/%s/bookings", orgRef)

	for _, m := range members {
		if _, err := q.InsertNotification(ctx, gen.InsertNotificationParams{
			RecipientID:      m.UserID,
			NotificationType: "service_booking_new",
			ActorType:        &actorType,
			ActorID:          bk.BookerUserID,
			SubjectType:      &subjectType,
			SubjectID:        bookingID,
			Title:            title,
			Body:             &body,
			ActionUrl:        &actionURL,
		}); err != nil {
			slog.Error("task:service:booking_created: notify member failed",
				"booking_id", bookingID, "member", m.UserID, "error", err)
		}
	}

	return nil
}

// HandleServiceBookingStatusChanged notifies the booker of an org/admin
// status transition (confirmed/declined/fulfilled/cancelled).
func HandleServiceBookingStatusChanged(ctx context.Context, t *asynq.Task) error {
	bookingID, status, _, _, err := bookingPayload(t)
	if err != nil {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}

	ntype := bookingNotificationType(status)
	if ntype == "" {
		return fmt.Errorf("%w: unknown status %q", asynq.SkipRetry, status)
	}

	q := gen.New(dbPool.Writer)

	bk, err := q.GetServiceBookingDetail(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: booking %s not found", asynq.SkipRetry, bookingID)
		}

		return fmt.Errorf("get booking: %w", err)
	}

	// GDPR-erased booker — no notification target.
	if !bk.BookerUserID.Valid {
		return nil
	}

	orgName := "the provider"
	if bk.OrgName != nil {
		orgName = *bk.OrgName
	}

	subjectType := "service_booking"
	actionURL := "/me/service-bookings/" + bookingID.String()
	title := fmt.Sprintf("Booking %s — %s", status, bk.ListingTitle)
	body := fmt.Sprintf("%s %s your booking for %s.", orgName, status, bk.ListingTitle)

	if bk.OrgResponseNote != nil && *bk.OrgResponseNote != "" {
		body += " Note: " + *bk.OrgResponseNote
	}

	if _, err := q.InsertNotification(ctx, gen.InsertNotificationParams{
		RecipientID:      bk.BookerUserID,
		NotificationType: ntype,
		ActorType:        strPtrWorker("organization"),
		ActorID:          bk.OrgID,
		SubjectType:      &subjectType,
		SubjectID:        bookingID,
		Title:            title,
		Body:             &body,
		ActionUrl:        &actionURL,
	}); err != nil {
		return fmt.Errorf("notify booker: %w", err)
	}

	return nil
}

// HandleServiceBookingFeeSettle is the critical-tier fee lifecycle task
// (§111): 'refund' returns the platform fee to the booker (declined,
// SLA-expired, org-cancelled, dispute-resolved-for-booker, in-window
// booker-cancel); 'recognize' marks the fee as platform revenue on
// 'fulfilled'; 'retain' is a forfeited booker-cancel. Idempotent —
// re-runs check the booking/payment state and no-op.
func HandleServiceBookingFeeSettle(ctx context.Context, t *asynq.Task) error {
	bookingID, _, action, _, err := bookingPayload(t)
	if err != nil {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}

	q := gen.New(dbPool.Writer)

	bk, err := q.GetServiceBooking(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: booking %s not found", asynq.SkipRetry, bookingID)
		}

		return fmt.Errorf("get booking: %w", err)
	}

	switch action {
	case "recognize", "retain":
		// Nothing money-moving — the fee payment already succeeded; on
		// 'fulfilled' the platform keeps it, on late booker-cancel it's
		// forfeited per §111 refund policy. No ledger field to flip in
		// this version — settlement is recorded by the booking status.
		return nil
	case "refund":
	default:
		return fmt.Errorf("%w: unknown fee action %q", asynq.SkipRetry, action)
	}

	// Idempotent: already-refunded bookings are terminal.
	if bk.Status == "refunded" {
		return nil
	}

	// Refunds only fire from refund-eligible statuses — a stale 'refund'
	// task must not claw back the fee from a booking that has since been
	// fulfilled, re-confirmed, or is still pending an org response
	// (mirrors MarkServiceBookingRefunded's source-status guard).
	switch bk.Status {
	case "declined", "cancelled", "disputed":
	default:
		return nil
	}

	if !bk.PaymentID.Valid {
		return nil // inquiry bookings carry no payment
	}

	payment, err := q.GetPaymentByID(ctx, bk.PaymentID)
	if err != nil {
		return fmt.Errorf("get payment %s: %w", bk.PaymentID, err)
	}

	if payment.Status != paymentStatusSucceeded {
		return nil // never captured — nothing to refund
	}

	if serviceBookingFeeRefunder == nil {
		return errors.New("service booking fee refunder not configured")
	}

	feeAmount := int64(0)
	if bk.FeeAmount != nil {
		feeAmount = *bk.FeeAmount
	}

	if err := serviceBookingFeeRefunder.RefundBookingFee(ctx, bk.PaymentID, feeAmount, "service booking "+bk.Status); err != nil {
		return fmt.Errorf("refund booking fee: %w", err)
	}

	// Reflect the refund on the booking row (declined|cancelled|disputed →
	// refunded). A guard failure just means an admin already moved it to
	// a terminal status — the payment refund is the source of truth.
	if _, err := q.MarkServiceBookingRefunded(ctx, bookingID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("task:service:booking_fee_settle: mark refunded failed",
			"booking_id", bookingID, "error", err)
	}

	return nil
}

// HandleServiceBookingExpirePending is the hourly low-tier sweep —
// declines pending bookings past expires_at (org response SLA) and
// triggers fee refunds where a fee payment exists.
func HandleServiceBookingExpirePending(ctx context.Context, t *asynq.Task) error {
	var p struct {
		Limit int32 `json:"limit"`
	}
	_ = json.Unmarshal(t.Payload(), &p) // empty payload → default batch

	if p.Limit <= 0 {
		p.Limit = 200
	}

	q := gen.New(dbPool.Writer)

	expired, err := q.ListExpiredPendingServiceBookings(ctx, p.Limit)
	if err != nil {
		return fmt.Errorf("list expired bookings: %w", err)
	}

	for _, bk := range expired {
		// DeclineServiceBooking is guarded on status='pending' — a racing
		// org response wins and this iteration no-ops.
		if _, err := q.DeclineServiceBooking(ctx, gen.DeclineServiceBookingParams{
			ID:              bk.ID,
			OrgResponseNote: strPtrWorker("Expired — the organization did not respond within the SLA window."),
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}

			slog.Error("task:service:booking_expire_pending: expire failed",
				"booking_id", bk.ID, "error", err)

			continue
		}

		// Booker notification.
		if bk.BookerUserID.Valid {
			subjectType := "service_booking"
			actionURL := "/me/service-bookings/" + bk.ID.String()
			body := "Your booking expired — the organization did not respond in time."

			if _, err := q.InsertNotification(ctx, gen.InsertNotificationParams{
				RecipientID:      bk.BookerUserID,
				NotificationType: "service_booking_declined",
				ActorType:        strPtrWorker("organization"),
				ActorID:          bk.OrgID,
				SubjectType:      &subjectType,
				SubjectID:        bk.ID,
				Title:            "Booking expired",
				Body:             &body,
				ActionUrl:        &actionURL,
			}); err != nil {
				slog.Error("task:service:booking_expire_pending: notify failed",
					"booking_id", bk.ID, "error", err)
			}
		}

		// Refund the platform fee if one was captured.
		if bk.PaymentID.Valid {
			payload, _ := json.Marshal(map[string]string{
				"booking_id": bk.ID.String(),
				"action":     "refund",
			})
			if err := taskEnqueuer.Enqueue(TypeServiceBookingFeeSettle, payload); err != nil {
				slog.Error("task:service:booking_expire_pending: fee settle enqueue failed",
					"booking_id", bk.ID, "error", err)
			}
		}
	}

	// §111 SLA nudge — pending bookings past half the response window get
	// a one-shot service_booking_reminder to the org (reminded_at marks
	// it; MarkServiceBookingReminded's guard makes the claim atomic).
	reminders, err := q.ListPendingBookingsDueReminder(ctx, p.Limit)
	if err != nil {
		return fmt.Errorf("list reminder-due bookings: %w", err)
	}

	for _, bk := range reminders {
		rows, err := q.MarkServiceBookingReminded(ctx, bk.ID)
		if err != nil {
			slog.Error("task:service:booking_expire_pending: mark reminded failed",
				"booking_id", bk.ID, "error", err)

			continue
		}

		if rows == 0 {
			continue // raced an org response or a duplicate sweep
		}

		members, err := q.ListOrganizationMembers(ctx, bk.OrgID)
		if err != nil {
			slog.Error("task:service:booking_expire_pending: list org members failed",
				"booking_id", bk.ID, "org_id", bk.OrgID, "error", err)

			continue
		}

		actorType := "system"
		subjectType := "service_booking"
		title := fmt.Sprintf("Response SLA halfway: %s", bk.ListingTitle)
		body := fmt.Sprintf("A %s for %s is still pending and half the response window has passed. Respond before it expires.", bk.Intent, bk.ListingTitle)

		orgRef := bk.OrgID.String()
		if bk.OrgSlug != "" {
			orgRef = bk.OrgSlug
		}

		actionURL := fmt.Sprintf("/org/%s/bookings", orgRef)

		for _, m := range members {
			if _, err := q.InsertNotification(ctx, gen.InsertNotificationParams{
				RecipientID:      m.UserID,
				NotificationType: "service_booking_reminder",
				ActorType:        &actorType,
				SubjectType:      &subjectType,
				SubjectID:        bk.ID,
				Title:            title,
				Body:             &body,
				ActionUrl:        &actionURL,
			}); err != nil {
				slog.Error("task:service:booking_expire_pending: reminder notify failed",
					"booking_id", bk.ID, "member", m.UserID, "error", err)
			}
		}
	}

	return nil
}

// strPtrWorker is a tiny helper — pointers into per-call literals.
func strPtrWorker(s string) *string { return &s }
