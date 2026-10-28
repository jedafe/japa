// fiat.go — §125 fiat tip settlement: creates a pending tip + a §17 payments
// row (purpose_type='donation'), returns the provider checkout URL. The
// worker-side settle/clawback tasks complete the flow.
package tips

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// createFiatTip validates bounds, inserts a pending tip, and initiates the
// provider checkout via the payments seam.
func (s *Service) createFiatTip(ctx context.Context, senderID pgtype.UUID, owner Owner, walletOwner pgtype.UUID, st Settings, in CreateTipInput, idemKey string) (CreateTipResult, error) {
	if s.payments == nil {
		return CreateTipResult{}, ErrInvalidMethod
	}
	if !st.AcceptFiat {
		return CreateTipResult{}, ErrMethodNotAccepted
	}

	amount := in.FiatAmount
	if amount <= 0 {
		amount = genericAmount(in)
	}
	if amount <= 0 {
		return CreateTipResult{}, ErrInvalidAmount
	}
	if s.settings != nil {
		if min := int64(s.settings.GetIntOr(ctx, "tips.min_fiat_minor", 100)); amount < min {
			return CreateTipResult{}, ErrBelowMinimum
		}
		if max := int64(s.settings.GetIntOr(ctx, "tips.max_fiat_minor", 1000000)); amount > max {
			return CreateTipResult{}, ErrAboveMaximum
		}
	}
	if amount < st.MinTipAmount {
		return CreateTipResult{}, ErrBelowMinimum
	}

	currency := in.FiatCurrency
	if currency == "" && s.settings != nil {
		currency = s.settings.GetStringOr(ctx, "wallet.default_currency", "NGN")
	}
	if currency == "" {
		currency = "NGN"
	}
	// Validate the currency up front — an unknown code would create a tip +
	// payment that can never settle (getCurrencyByCode fails inside the
	// settle task and retries to the DLQ).
	cur, err := s.store.getCurrencyByCode(ctx, currency)
	if err != nil {
		return CreateTipResult{}, ErrInvalidAmount
	}

	// Quote the conversion + fee at create time so the sender's promised
	// amount is what settles — settle uses the stored net rather than live
	// settings (§125 formula, applied once).
	var feeCredits, netCredits int64
	{
		exp := currencyExponent(currency)
		div := int64(1)
		for i := 0; i < exp; i++ {
			div *= 10
		}
		gross := amount * int64(cur.CreditsPerUnit) / div
		feeCredits = gross * int64(s.feePercent(ctx)) / 100
		netCredits = gross - feeCredits
	}

	tip, err := s.store.insertTip(ctx, gen.InsertTipParams{
		SenderUserID:            senderID,
		RecipientType:           owner.Type,
		RecipientUserID:         nullableOwnerUser(owner),
		RecipientOrganizationID: nullableOwnerOrg(owner),
		ContentID:               in.ContentID,
		Method:                  in.Method,
		FiatAmount:              &amount,
		FiatCurrency:            &currency,
		FeeAmount:               feeCredits,
		NetCredits:              netCredits,
		Message:                 strPtrOrNil(in.Message),
		IsAnonymous:             in.IsAnonymous,
		Status:                  StatusPending,
		IdempotencyKey:          strPtrOrNil(idemKey),
	})
	if err != nil {
		// tips_idem_unique (000949): concurrent same-key replay.
		if errors.Is(err, ErrDuplicateKey) && idemKey != "" {
			if existing, gerr := s.store.getTipByIdempotencyKey(ctx, senderID, idemKey); gerr == nil {
				return CreateTipResult{Tip: tipFromRow(existing), Duplicate: true}, nil
			}
		}
		return CreateTipResult{}, err
	}

	// Paystack requires the payer email at /transaction/initialize; resolve it
	// from the sender's user row (best-effort — providers that don't need it
	// ignore the field).
	var email string
	if e, err := s.store.getUserEmail(ctx, senderID); err == nil {
		email = e
	}

	pay, err := s.payments.CreatePayment(ctx, senderID, PaymentCreateInput{
		Provider:    in.Method,
		PurposeType: "donation",
		PurposeID:   tip.ID.String(),
		Amount:      amount,
		Currency:    currency,
		Email:       email,
		Metadata: map[string]interface{}{
			"tip_id":         tip.ID.String(),
			"recipient_type": owner.Type,
		},
	})
	if err != nil {
		return CreateTipResult{}, err
	}

	var paymentID pgtype.UUID
	if err := paymentID.Scan(pay.ID); err == nil {
		if err := s.store.attachTipPayment(ctx, tip.ID, paymentID); err != nil {
			slog.Warn("tips: failed to attach payment to tip", "tip_id", tip.ID.String(), "payment_id", pay.ID, "error", err)
		}
	}

	return CreateTipResult{Tip: tipFromRow(tip), CheckoutURL: pay.CheckoutURL}, nil
}
