// settle_svc.go — §125 post-payment settlement and refund/clawback service
// paths, invoked by the worker tasks tips:settle_fiat and tips:clawback plus
// the admin refund route.
package tips

import (
	"context"
	"errors"
	"fmt"

	"github.com/jedafe/japa/internal/db/gen"

	"github.com/jackc/pgx/v5/pgtype"
)

// NotificationCreator dispatches tip.received notifications — satisfied by
// *notifications.Service.
type NotificationCreator interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// SettleFiatTip converts a succeeded donation payment into recipient credits:
// gross = fiat_amount / 10^exp × credits_per_unit; net = gross − fee.
// Idempotent — the store no-ops on an already-completed tip.
func (s *Service) SettleFiatTip(ctx context.Context, tipID pgtype.UUID) error {
	tip, err := s.store.getTip(ctx, tipID)
	if err != nil {
		return err
	}
	if tip.Status == StatusCompleted {
		return nil
	}
	if tip.Status != StatusPending {
		return ErrInvalidStatus
	}

	walletOwner, err := s.walletOwnerFor(ctx, tip)
	if err != nil {
		return err
	}

	var gross int64
	if tip.FiatAmount != nil && tip.FiatCurrency != nil {
		cur, err := s.store.getCurrencyByCode(ctx, *tip.FiatCurrency)
		if err != nil {
			return fmt.Errorf("resolving currency %s: %w", *tip.FiatCurrency, err)
		}
		exp := currencyExponent(*tip.FiatCurrency)
		div := int64(1)
		for i := 0; i < exp; i++ {
			div *= 10
		}
		gross = *tip.FiatAmount * int64(cur.CreditsPerUnit) / div
	}
	fee := gross * int64(s.feePercent(ctx)) / 100
	net := gross - fee

	if err := s.store.settleFiatTip(ctx, tipID, net, walletOwner); err != nil {
		return err
	}
	s.notifyReceived(ctx, tipID)
	return nil
}

// ClawbackTip reverses a settled tip's recipient credits and marks it
// refunded — the §125 refund path (payment refunded upstream or admin).
func (s *Service) ClawbackTip(ctx context.Context, tipID pgtype.UUID) error {
	tip, err := s.store.getTip(ctx, tipID)
	if err != nil {
		return err
	}
	if tip.Status == StatusRefunded {
		return nil
	}
	if tip.Status == StatusPending {
		// Refund arrived before settlement — nothing to claw back, just
		// mark refunded so the refund webhook doesn't retry forever.
		return s.store.updateTipStatus(ctx, tipID, StatusRefunded)
	}
	if tip.Status != StatusCompleted {
		return ErrInvalidStatus
	}
	walletOwner, err := s.walletOwnerFor(ctx, tip)
	if err != nil {
		return err
	}
	return s.store.clawbackTip(ctx, tipID, walletOwner)
}

// AdminRefundTip is the admin moderation path — tips.moderate is enforced at
// the route layer; the payment-side provider refund runs through §17's own
// refund flow, which enqueues tips:clawback via the payment-status hook.
func (s *Service) AdminRefundTip(ctx context.Context, tipID, _ pgtype.UUID) error {
	return s.ClawbackTip(ctx, tipID)
}

// walletOwnerFor resolves which user's credits wallet receives a settlement —
// the recipient user, or the org's current owner. Platform tips have no wallet.
func (s *Service) walletOwnerFor(ctx context.Context, tip gen.Tip) (pgtype.UUID, error) {
	switch tip.RecipientType {
	case RecipientUser:
		return tip.RecipientUserID, nil
	case RecipientOrganization:
		return s.store.getOrgOwnerID(ctx, tip.RecipientOrganizationID)
	case RecipientPlatform:
		return pgtype.UUID{}, nil
	default:
		return pgtype.UUID{}, errors.New("unknown recipient type")
	}
}

// currencyExponent maps ISO-4217 minor-unit exponents (JPY=0; default 2).
func currencyExponent(code string) int {
	switch code {
	case "JPY", "KRW", "VND", "XOF", "XAF":
		return 0
	case "BHD", "JOD", "KWD", "OMR":
		return 3
	default:
		return 2
	}
}

// NotifyTipReceived is the tips:notify_received task handler — dispatches the
// billing-category tip.received notification to the recipient (or org owner).
func (s *Service) NotifyTipReceived(ctx context.Context, tipID pgtype.UUID, notify NotificationCreator) error {
	tip, err := s.store.getTip(ctx, tipID)
	if err != nil {
		return err
	}
	recipient, err := s.walletOwnerFor(ctx, tip)
	if err != nil {
		return err
	}
	if !recipient.Valid || notify == nil {
		return nil
	}
	amount := tipDisplayAmount(tip)
	sender := senderName(tip)
	if sender == "" {
		sender = "A member"
	}
	title := fmt.Sprintf("%s tipped you %s", sender, amount)
	var body string
	if tip.Message != nil {
		body = *tip.Message
	}
	actionURL := "/me/tips/received"
	// Anonymous tips keep the sender identity out of the notification actor —
	// the actor field is exposed to notification renderers and would leak it.
	var actorID pgtype.UUID
	if !tip.IsAnonymous {
		actorID = tip.SenderUserID
	}
	return notify.CreateNotification(ctx, recipient, "tip.received", "user", actorID, "tip", tip.ID, title, body, actionURL)
}

// tipDisplayAmount renders a human amount for notifications.
func tipDisplayAmount(tip gen.Tip) string {
	switch {
	case tip.Method == MethodPoints:
		return fmt.Sprintf("%d points", tip.PointsAmount)
	case tip.Method == MethodCredits:
		return fmt.Sprintf("%d credits", tip.CreditsAmount)
	case tip.FiatAmount != nil && tip.FiatCurrency != nil:
		return fmt.Sprintf("%.2f %s", float64(*tip.FiatAmount)/100.0, *tip.FiatCurrency)
	default:
		return "a tip"
	}
}
