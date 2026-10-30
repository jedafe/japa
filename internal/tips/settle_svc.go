// settle_svc.go — §125 post-payment settlement and refund/clawback service
// paths, invoked by the worker tasks tips:settle_fiat and tips:clawback plus
// the admin refund route.
package tips

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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
// Idempotent — the store no-ops on an already-completed tip. paymentID is the
// §17 payment that triggered settlement; it repairs tip.payment_id when the
// create-time attach raced/failed.
func (s *Service) SettleFiatTip(ctx context.Context, tipID, paymentID pgtype.UUID) error {
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

	// Net was quoted at create — settle exactly what the sender was promised.
	// Rows predating quote-at-create (net=0, e.g. legacy pending tips) fall
	// back to the live rate so they still settle.
	net := tip.NetCredits
	if net == 0 && tip.FiatAmount != nil && tip.FiatCurrency != nil {
		cur, err := s.store.getCurrencyByCode(ctx, *tip.FiatCurrency)
		if err != nil {
			return fmt.Errorf("resolving currency %s: %w", *tip.FiatCurrency, err)
		}
		exp := currencyExponent(*tip.FiatCurrency)
		div := int64(1)
		for i := 0; i < exp; i++ {
			div *= 10
		}
		gross := *tip.FiatAmount * int64(cur.CreditsPerUnit) / div
		fee := gross * int64(s.feePercent(ctx)) / 100
		net = gross - fee
	}

	if err := s.store.settleFiatTip(ctx, tipID, net, walletOwner); err != nil {
		return err
	}
	if !tip.PaymentID.Valid && paymentID.Valid {
		if err := s.store.attachTipPayment(ctx, tipID, paymentID, ""); err != nil {
			slog.Warn("tips: failed to repair payment link", "tip_id", tipID.String(), "error", err)
		}
	}
	s.notifyReceived(ctx, tipID)
	return nil
}

// FailTip marks a pending tip failed — the payment-failed/expired branch of
// the fiat lifecycle. Idempotent: only pending rows transition.
func (s *Service) FailTip(ctx context.Context, tipID pgtype.UUID) error {
	return s.store.failTip(ctx, tipID)
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
	if err := s.store.clawbackTip(ctx, tipID, walletOwner); err != nil {
		return err
	}
	// The recipient's wallet was debited — they must know why, especially
	// when the clawback pushes them negative (debt-style recovery).
	s.notifyClawback(ctx, tip, walletOwner)
	return nil
}

// notifyClawback tells the wallet owner their tip was refunded/clawed back.
// Synchronous single insert is fine — this path runs from a worker task or
// an admin action, not a latency-sensitive send.
func (s *Service) notifyClawback(ctx context.Context, tip gen.Tip, walletOwner pgtype.UUID) {
	if s.clawNotifier == nil || !walletOwner.Valid {
		return
	}
	unit := "credits"
	if tip.Method == MethodPoints {
		unit = "points"
	}
	if err := s.clawNotifier.CreateNotification(ctx, walletOwner,
		"tip.received", "system", pgtype.UUID{},
		"tip", tip.ID,
		"Tip refunded",
		"A tip you received was refunded and the "+unit+" were removed from your wallet.",
		"/me/tips"); err != nil {
		slog.Warn("tips: clawback notification failed", "tip_id", tip.ID.String(), "error", err)
	}
}

// AdminRefundTip is the admin moderation path — tips.moderate is enforced at
// the route layer. For fiat tips the donor's payment is refunded provider-side
// first (§17 CreateRefund, which marks the payment refunded and enqueues the
// webhook reversal — the clawback here is the idempotent immediate version of
// that hook); for wallet/OC tips only the ledger clawback runs. Every call
// writes an audit_logs row.
func (s *Service) AdminRefundTip(ctx context.Context, tipID, adminID pgtype.UUID) error {
	tip, err := s.store.getTip(ctx, tipID)
	if err != nil {
		return err
	}
	// Provider-side refund first: without it the donor loses the fiat AND the
	// recipient's credits are clawed back — the platform would keep both.
	// Pending tips get a best-effort attempt too: a payment may have succeeded
	// before the settle task ran (it DLQs harmlessly once the tip is refunded).
	// A failed attempt on a pending tip means the payment was never
	// chargeable — the clawback below is still the right local action.
	if IsFiatMethod(tip.Method) && tip.PaymentID.Valid && tip.Status != StatusRefunded && s.refunder == nil {
		// Belt-and-suspenders: an un-wired refunder must not silently
		// reproduce the clawback-without-donor-refund asymmetry.
		if tip.Status == StatusCompleted {
			return errors.New("tips: payment refunder seam not configured")
		}
		slog.Warn("tips: refund without provider refund — refunder seam not wired", "tip_id", tipID.String())
	}
	if IsFiatMethod(tip.Method) && tip.PaymentID.Valid && tip.Status != StatusRefunded && s.refunder != nil {
		amount := int64(0)
		if tip.FiatAmount != nil {
			amount = *tip.FiatAmount
		}
		if err := s.refunder.RefundPayment(ctx, tip.PaymentID.String(), amount, "admin tip refund"); err != nil {
			if tip.Status == StatusCompleted {
				return fmt.Errorf("provider refund for tip %s: %w", tipID.String(), err)
			}
			slog.Warn("tips: provider refund skipped for pending tip", "tip_id", tipID.String(), "error", err)
		}
	}
	if err := s.ClawbackTip(ctx, tipID); err != nil {
		return err
	}
	if err := s.store.insertAuditLog(ctx, adminID, "tips.admin_refund", "tip", tipID.String(),
		fmt.Sprintf(`{"tip_id":%q,"method":%q,"status":%q}`, tipID.String(), tip.Method, tip.Status)); err != nil {
		slog.Warn("tips: refund audit log failed", "tip_id", tipID.String(), "error", err)
	}
	return nil
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
	// §125 payload — content title when the tip is content-anchored.
	if tip.ContentID.Valid {
		if ct, err := s.store.getContentTitle(ctx, tip.ContentID); err == nil && ct != "" {
			body = "on " + ct
		}
	}
	if tip.Message != nil {
		if body != "" {
			body += " — "
		}
		body += *tip.Message
	}
	actionURL := "/me/tips"
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

// SweepStalePending fails pending tips past tips.pending_ttl_hours whose
// payment can no longer charge the donor — the TTL janitor for abandoned
// checkouts (default 48h; provider sessions die well before that).
func (s *Service) SweepStalePending(ctx context.Context) error {
	hours := int32(48)
	if s.settings != nil {
		if h := s.settings.GetIntOr(ctx, "tips.pending_ttl_hours", 48); h > 0 {
			hours = int32(h)
		}
	}
	ids, err := s.store.listStalePendingTips(ctx, hours, 500)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.store.failTip(ctx, id); err != nil {
			slog.Warn("tips: stale sweep failTip", "tip_id", id.String(), "error", err)
		}
	}
	return nil
}
