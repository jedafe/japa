// booking_fee.go — adapter exposing the §17 refund path to the worker
// package's service:booking_fee_settle task (§111). The worker declares a
// narrow ServiceBookingFeeRefunder interface; this adapter satisfies it
// against *payments.Service. Worker-side import direction is preserved
// (payments already imports worker — the adapter lives here so servicedir
// → worker → adapter never forms a cycle).
//
// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
package payments

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

// BookingFeeRefunder adapts *Service to the worker's
// ServiceBookingFeeRefunder interface for platform-fee refunds.
type BookingFeeRefunder struct {
	svc *Service
}

// NewBookingFeeRefunder wraps the payments service.
func NewBookingFeeRefunder(svc *Service) *BookingFeeRefunder {
	return &BookingFeeRefunder{svc: svc}
}

// RefundBookingFee refunds the platform-fee payment for a service
// booking. Idempotent: if the payment is already refunded (or was never
// captured) the call is a no-op.
func (r *BookingFeeRefunder) RefundBookingFee(ctx context.Context, paymentID pgtype.UUID, amount int64, reason string) error {
	if r == nil || r.svc == nil {
		return errors.New("payments: booking fee refunder not initialized")
	}

	p, err := r.svc.store.getPaymentByID(ctx, paymentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // nothing to refund
		}

		return err
	}

	if p.Status == PaymentStatusRefunded {
		return nil // already refunded — idempotent
	}

	if p.Status != PaymentStatusSucceeded {
		return nil // never captured — nothing to refund
	}

	if amount <= 0 {
		amount = p.Amount
	}

	_, err = r.svc.CreateRefund(ctx, CreateRefundInput{
		PaymentID: paymentID.String(),
		Amount:    amount,
		Reason:    reason,
	})

	return err
}

// enqueueServiceBookingCreated fires service:booking_created for a
// service_booking_fee payment that reached 'succeeded' synchronously —
// credit-wallet debits and zero-amount (fully discounted / zero-fee)
// payments never pass through a provider webhook, so the worker's
// onPaymentSucceeded dispatch would never see them and the org would
// never be notified. PurposeID carries the booking UUID. The task is
// idempotent-keyed on (type, payload) so a later provider webhook for
// the same payment dedupes.
func (s *Service) enqueueServiceBookingCreated(p gen.Payment) {
	if s == nil || s.enqueuer == nil ||
		p.PurposeType != PurposeServiceBookingFee || !p.PurposeID.Valid {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"booking_id": p.PurposeID.String(),
		"payment_id": p.ID.String(),
	})
	if err := s.enqueuer.Enqueue(worker.TypeServiceBookingCreated, payload); err != nil {
		slog.Error("payments: enqueue booking_created for sync payment failed",
			"payment_id", p.ID, "booking_id", p.PurposeID, "error", err)
	}
}
