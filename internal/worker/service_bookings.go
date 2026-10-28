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
	case "cancelled", "refunded":
		return "service_booking_cancelled"
	case "disputed":
		return "service_booking_disputed"
	}

	return ""
}

// bookingTaskPayload is the decoded payload shared by the §111 booking
// tasks. audience selects the notification target for status_changed:
// "" → booker (default, provider/admin transitions — actor is the org or
// provider user), "org" → org members (booker-raised disputes),
// "provider" → the provider side, actor = booker (booker-initiated
// cancel/complete — a booker never self-notifies "provider cancelled").
type bookingTaskPayload struct {
	BookingID pgtype.UUID
	Status    string
	Action    string
	PaymentID pgtype.UUID
	Audience  string
}

// bookingPayload extracts the booking fields from a task payload.
// Permanent decode failures are skipped, not retried.
func bookingPayload(t *asynq.Task) (bookingTaskPayload, error) {
	var p struct {
		BookingID string `json:"booking_id"`
		Status    string `json:"status"`
		Action    string `json:"action"`
		PaymentID string `json:"payment_id"`
		Audience  string `json:"audience"`
	}

	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return bookingTaskPayload{}, fmt.Errorf("decode payload: %w", err)
	}

	if p.BookingID == "" {
		return bookingTaskPayload{}, errors.New("missing booking_id")
	}

	out := bookingTaskPayload{Status: p.Status, Action: p.Action, Audience: p.Audience}
	if err := out.BookingID.Scan(p.BookingID); err != nil {
		return bookingTaskPayload{}, fmt.Errorf("invalid booking_id: %w", err)
	}

	if p.PaymentID != "" {
		if err := out.PaymentID.Scan(p.PaymentID); err != nil {
			return bookingTaskPayload{}, fmt.Errorf("invalid payment_id: %w", err)
		}
	}

	return out, nil
}

// HandleServiceBookingCreated fans out a "new booking" notification to
// every member of the selling org (service_booking_new). Runs for inquiry
// bookings at creation time and for paid bookings only after the fee
// payment succeeds — the enqueue sites enforce that ordering.
func HandleServiceBookingCreated(ctx context.Context, t *asynq.Task) error {
	p, err := bookingPayload(t)
	if err != nil {
		return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
	}

	bookingID := p.BookingID
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
	if p.PaymentID.Valid && !bk.PaymentID.Valid {
		if _, err := q.BackfillServiceBookingPayment(ctx, gen.BackfillServiceBookingPaymentParams{
			ID: bookingID, PaymentID: p.PaymentID,
		}); err != nil {
			slog.Error("task:service:booking_created: payment backfill failed",
				"booking_id", bookingID, "payment_id", p.PaymentID, "error", err)
		} else {
			bk.PaymentID = p.PaymentID
		}
	}

	// The succeeded payment must be the booking's own linked payment —
	// a success event naming this booking via purpose_id but carrying a
	// different payment must not notify the org of a "paid" booking whose
	// fee never actually landed.
	if p.PaymentID.Valid && bk.PaymentID.Valid && bk.PaymentID != p.PaymentID {
		return nil
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

	actorType := "user"
	title := fmt.Sprintf("New %s: %s", map[bool]string{true: "booking", false: "inquiry"}[bk.Intent == "booking"], bk.ListingTitle)
	body := fmt.Sprintf("A %s was submitted for %s. Respond before the SLA expires.", bk.Intent, bk.ListingTitle)

	// 000824 user-provider (mentorship) rows carry no response SLA —
	// expires_at stays NULL so the expiry/reminder sweeps never touch
	// them; the copy must not promise a deadline that doesn't exist.
	if !bk.OrgID.Valid && bk.ProviderUserID.Valid {
		body = "A new mentorship request is waiting for your response."
	}

	return notifyOrgMembers(ctx, q, bk, "service_booking_new", &actorType, bk.BookerUserID, title, body)
}

// notifyOrgMembers fans out a notification to every member of the
// booking's selling org — or, on 000824 user-provider (mentorship)
// bookings where org_id is NULL, directly to the provider user. Used by
// booking_created (new booking), the status_changed audience=org path
// (booker-raised dispute), and audience=provider (booker-initiated
// cancel/complete).
func notifyOrgMembers(ctx context.Context, q *gen.Queries, bk gen.GetServiceBookingDetailRow, ntype string, actorType *string, actorID pgtype.UUID, title, body string) error {
	// Provider arm: no org row exists to fan out to — the provider user
	// is the entire audience. The action URL targets the mentorship
	// inbox's RECEIVED arm — /messages/mentorship defaults to role=sent
	// (the mentee view), so providers need the explicit role param.
	if !bk.OrgID.Valid && bk.ProviderUserID.Valid {
		subjectType := "service_booking"
		actionURL := "/messages/mentorship?role=received&b=" + bk.ID.String()

		if _, err := q.InsertNotification(ctx, gen.InsertNotificationParams{
			RecipientID:      bk.ProviderUserID,
			NotificationType: ntype,
			ActorType:        actorType,
			ActorID:          actorID,
			SubjectType:      &subjectType,
			SubjectID:        bk.ID,
			Title:            title,
			Body:             &body,
			ActionUrl:        &actionURL,
		}); err != nil {
			return fmt.Errorf("notify provider user: %w", err)
		}

		return nil
	}

	members, err := q.ListOrganizationMembers(ctx, gen.ListOrganizationMembersParams{OrganizationID: bk.OrgID})
	if err != nil {
		return fmt.Errorf("list org members: %w", err)
	}

	subjectType := "service_booking"

	orgRef := bk.OrgID.String()
	if bk.OrgSlug != nil && *bk.OrgSlug != "" {
		orgRef = *bk.OrgSlug
	}

	actionURL := fmt.Sprintf("/org/%s/service-bookings", orgRef)

	for _, m := range members {
		if _, err := q.InsertNotification(ctx, gen.InsertNotificationParams{
			RecipientID:      m.UserID,
			NotificationType: ntype,
			ActorType:        actorType,
			ActorID:          actorID,
			SubjectType:      &subjectType,
			SubjectID:        bk.ID,
			Title:            title,
			Body:             &body,
			ActionUrl:        &actionURL,
		}); err != nil {
			slog.Error("servicedir worker: notify org member failed",
				"booking_id", bk.ID, "type", ntype, "member", m.UserID, "error", err)
		}
	}

	return nil
}

// HandleServiceBookingStatusChanged notifies the booker of an org/admin
// status transition (confirmed/declined/fulfilled/cancelled). Payload
// audience="org" inverts the direction — the booker raised a dispute, so
// org members get service_booking_disputed instead. audience="provider"
// sends the alert to the provider side on booker-initiated transitions
// (mentee cancel/complete) with actor=booker.
func HandleServiceBookingStatusChanged(ctx context.Context, t *asynq.Task) error {
	p, err := bookingPayload(t)
	if err != nil {
		return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
	}

	bookingID := p.BookingID
	q := gen.New(dbPool.Writer)

	bk, err := q.GetServiceBookingDetail(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: booking %s not found", asynq.SkipRetry, bookingID)
		}

		return fmt.Errorf("get booking: %w", err)
	}

	if p.Audience == "org" {
		// Booker-raised dispute — org-facing notification. Guard on the
		// row's actual state so a stale task can't misreport.
		if bk.Status != "disputed" {
			return nil
		}

		title := "Dispute raised: " + bk.ListingTitle

		body := fmt.Sprintf("The booker reported a problem with their %s on %s. An admin will review the dispute.", bk.Intent, bk.ListingTitle)
		if bk.DisputeReason != nil && *bk.DisputeReason != "" {
			body += " Reason: " + *bk.DisputeReason
		}

		return notifyOrgMembers(ctx, q, bk, "service_booking_disputed", strPtrWorker("user"), bk.BookerUserID, title, body)
	}

	status := p.Status
	ntype := bookingNotificationType(status)

	if ntype == "" {
		return fmt.Errorf("%w: unknown status %q", asynq.SkipRetry, status)
	}

	if p.Audience == "provider" {
		// Booker-initiated transition (booker cancel, mentee complete) —
		// the provider side is the audience, the booker is the actor. The
		// mentee must never self-notify "provider cancelled"; the mentor /
		// org members need the ping. notifyOrgMembers resolves the target:
		// provider_user_id on user-provider rows, org members on org rows.
		bookerName := "The booker"
		if bk.BookerDisplayName != nil && *bk.BookerDisplayName != "" {
			bookerName = *bk.BookerDisplayName
		}

		title := fmt.Sprintf("Booking %s — %s", status, bk.ListingTitle)

		body := fmt.Sprintf("%s %s their %s for %s.", bookerName, status, bk.Intent, bk.ListingTitle)
		if bk.CancelReason != nil && *bk.CancelReason != "" {
			body += " Reason: " + *bk.CancelReason
		}

		return notifyOrgMembers(ctx, q, bk, ntype, strPtrWorker("user"), bk.BookerUserID, title, body)
	}

	// GDPR-erased booker — no notification target.
	if !bk.BookerUserID.Valid {
		return nil
	}

	orgName := "the provider"
	if bk.OrgName != nil {
		orgName = *bk.OrgName
	}

	// 000824: user-provider (mentorship) rows have no org actor — the
	// responding mentor is the actor and supplies the body copy name.
	actorType := "organization"
	actorID := bk.OrgID

	if !bk.OrgID.Valid && bk.ProviderUserID.Valid {
		actorType = "user"
		actorID = bk.ProviderUserID

		if bk.ProviderDisplayName != nil && *bk.ProviderDisplayName != "" {
			orgName = *bk.ProviderDisplayName
		}
	}

	subjectType := "service_booking"

	actionURL := "/messages/bookings?b=" + bookingID.String()
	if bk.ListingKind == "mentorship" {
		// The mentee's requests live on the mentorship inbox — whose
		// role=sent default is already the booker arm, no param needed.
		actionURL = "/messages/mentorship?b=" + bookingID.String()
	}

	title := fmt.Sprintf("Booking %s — %s", status, bk.ListingTitle)
	body := fmt.Sprintf("%s %s your booking for %s.", orgName, status, bk.ListingTitle)

	if bk.OrgResponseNote != nil && *bk.OrgResponseNote != "" {
		body += " Note: " + *bk.OrgResponseNote
	}

	if _, err := q.InsertNotification(ctx, gen.InsertNotificationParams{
		RecipientID:      bk.BookerUserID,
		NotificationType: ntype,
		ActorType:        &actorType,
		ActorID:          actorID,
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
	p, err := bookingPayload(t)
	if err != nil {
		return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
	}

	bookingID := p.BookingID
	action := p.Action

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

	// Refunds only fire from refund-eligible statuses — a stale 'refund'
	// task must not claw back the fee from a booking that has since been
	// fulfilled, re-confirmed, or is still pending an org response
	// (mirrors MarkServiceBookingRefunded's source-status guard). The
	// 'refunded' status is included: the admin path stamps it *before*
	// this task runs, so the payment may still be 'succeeded' — the real
	// idempotency signal is payment.Status below (a second run sees
	// 'refunded' and no-ops).
	switch bk.Status {
	case "declined", "cancelled", "disputed", "refunded":
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
			actionURL := "/messages/bookings?b=" + bk.ID.String()
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

		members, err := q.ListOrganizationMembers(ctx, gen.ListOrganizationMembersParams{OrganizationID: bk.OrgID})
		if err != nil {
			slog.Error("task:service:booking_expire_pending: list org members failed",
				"booking_id", bk.ID, "org_id", bk.OrgID, "error", err)

			continue
		}

		actorType := "system"
		subjectType := "service_booking"
		title := "Response SLA halfway: " + bk.ListingTitle
		body := fmt.Sprintf("A %s for %s is still pending and half the response window has passed. Respond before it expires.", bk.Intent, bk.ListingTitle)

		orgRef := bk.OrgID.String()
		if bk.OrgSlug != "" {
			orgRef = bk.OrgSlug
		}

		actionURL := fmt.Sprintf("/org/%s/service-bookings", orgRef)

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
