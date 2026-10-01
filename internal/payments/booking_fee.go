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
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
