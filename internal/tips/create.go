// create.go — §125 POST /tips: recipient resolution, validation, and the
// points/credits settlement path (fiat branches into fiat.go).
package tips

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// CreateTip validates and settles a tip. points/credits settle synchronously
// via the wallet ledger pair; fiat methods create a pending tip + payment row
// (fiat.go) and return a checkout URL.
func (s *Service) CreateTip(ctx context.Context, senderID pgtype.UUID, in CreateTipInput, idempotencyKey string) (CreateTipResult, error) {
	if s.settings != nil && !s.featureEnabled(ctx) {
		return CreateTipResult{}, ErrTipsDisabled
	}
	if senderID.Valid == false {
		return CreateTipResult{}, ErrInvalidRecipient
	}

	// Idempotency replay — return the original tip instead of double-settling.
	if idempotencyKey != "" {
		if existing, err := s.store.getTipByIdempotencyKey(ctx, senderID, idempotencyKey); err == nil {
			return CreateTipResult{Tip: tipFromRow(existing), Duplicate: true}, nil
		}
	}

	if s.perms != nil && !s.perms.Can(ctx, senderID, "tips.send") {
		return CreateTipResult{}, ErrNotEligible
	}

	// Resolve the recipient into an owner + the wallet user who receives value.
	owner, walletOwner, err := s.resolveRecipient(ctx, in)
	if err != nil {
		return CreateTipResult{}, err
	}

	// Self-tip guard: direct, via content author, or via org ownership.
	if owner.Type == RecipientUser && owner.ID == senderID {
		return CreateTipResult{}, ErrSelfTip
	}
	if walletOwner == senderID {
		return CreateTipResult{}, ErrSelfTip
	}

	st, err := s.store.getSettings(ctx, owner.Type, owner.ID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return CreateTipResult{}, ErrRecipientNoTips
		}
		return CreateTipResult{}, err
	}
	if !st.TipsEnabled {
		return CreateTipResult{}, ErrRecipientNoTips
	}
	sett := s.toSettings(st)

	switch in.Method {
	case MethodPoints, MethodCredits:
		return s.createWalletTip(ctx, senderID, owner, walletOwner, sett, in, idempotencyKey)
	default:
		if IsFiatMethod(in.Method) {
			return s.createFiatTip(ctx, senderID, owner, walletOwner, sett, in, idempotencyKey)
		}
		return CreateTipResult{}, ErrInvalidMethod
	}
}

// resolveRecipient maps the input recipient to a tip_settings owner plus the
// user whose wallet receives credits (org tips land on the org owner;
// platform tips have no wallet owner).
func (s *Service) resolveRecipient(ctx context.Context, in CreateTipInput) (Owner, pgtype.UUID, error) {
	// Content-anchored tip: recipient is the content's author.
	if in.ContentID.Valid {
		author, err := s.store.getContentAuthorID(ctx, in.ContentID)
		if err != nil {
			return Owner{}, pgtype.UUID{}, ErrNotFound
		}
		return UserOwner(author), author, nil
	}

	switch in.RecipientType {
	case RecipientUser:
		if !in.RecipientUserID.Valid {
			return Owner{}, pgtype.UUID{}, ErrInvalidRecipient
		}
		return UserOwner(in.RecipientUserID), in.RecipientUserID, nil
	case RecipientOrganization:
		if !in.RecipientOrgID.Valid {
			return Owner{}, pgtype.UUID{}, ErrInvalidRecipient
		}
		ownerID, err := s.store.getOrgOwnerID(ctx, in.RecipientOrgID)
		if err != nil {
			return Owner{}, pgtype.UUID{}, ErrNotFound
		}
		return OrgOwner(in.RecipientOrgID), ownerID, nil
	case RecipientPlatform:
		return PlatformOwner(), pgtype.UUID{}, nil
	default:
		return Owner{}, pgtype.UUID{}, ErrInvalidRecipient
	}
}

// createWalletTip runs the synchronous points/credits path.
func (s *Service) createWalletTip(ctx context.Context, senderID pgtype.UUID, owner Owner, walletOwner pgtype.UUID, st Settings, in CreateTipInput, idemKey string) (CreateTipResult, error) {
	// Platform has no wallet — points/credits tips to it are invalid.
	if owner.Type == RecipientPlatform {
		return CreateTipResult{}, ErrMethodNotAccepted
	}
	// Orgs without a resolved owner wallet can't settle either.
	if !walletOwner.Valid {
		return CreateTipResult{}, ErrInvalidRecipient
	}

	var amount int64
	var ledgerType string
	var accepted bool
	if in.Method == MethodPoints {
		amount, ledgerType, accepted = in.PointsAmount, "points", st.AcceptPoints
		if in.PointsAmount <= 0 && in.CreditsAmount <= 0 && in.FiatAmount <= 0 {
			amount = genericAmount(in)
		}
	} else {
		amount, ledgerType, accepted = in.CreditsAmount, "credits", st.AcceptCredits
		if in.CreditsAmount <= 0 && in.PointsAmount <= 0 && in.FiatAmount <= 0 {
			amount = genericAmount(in)
		}
	}
	if amount <= 0 {
		return CreateTipResult{}, ErrInvalidAmount
	}
	if !accepted {
		return CreateTipResult{}, ErrMethodNotAccepted
	}
	if amount < st.MinTipAmount {
		return CreateTipResult{}, ErrBelowMinimum
	}

	// Points tips enforce the daily anti-laundering cap.
	if in.Method == MethodPoints && s.settings != nil {
		limit := int64(s.settings.GetIntOr(ctx, "tips.points_daily_limit", 500))
		if limit > 0 {
			today, err := s.store.sumTipperPointsToday(ctx, senderID)
			if err != nil {
				return CreateTipResult{}, err
			}
			if today+amount > limit {
				return CreateTipResult{}, ErrDailyPointsLimit
			}
		}
	}

	// Fee applies to credits (and fiat) only — points pass through whole.
	var fee int64
	if in.Method == MethodCredits {
		fee = amount * int64(s.feePercent(ctx)) / 100
	}
	net := amount - fee

	tip, err := s.store.settleWalletTip(ctx, settleWalletParams{
		senderID:        senderID,
		walletOwnerID:   walletOwner,
		recipientUserID: nullableOwnerUser(owner),
		recipientOrgID:  nullableOwnerOrg(owner),
		ledgerType:      ledgerType,
		gross:           amount,
		net:             net,
		fee:             fee,
		method:          in.Method,
		recipientType:   owner.Type,
		contentID:       in.ContentID,
		message:         in.Message,
		isAnonymous:     in.IsAnonymous,
		idempotencyKey:  idemKey,
	})
	if err != nil {
		return CreateTipResult{}, err
	}

	s.notifyReceived(ctx, tip.ID)
	return CreateTipResult{Tip: tipFromRow(tip)}, nil
}

// genericAmount maps the handler's single `amount` field onto per-method
// columns when the caller didn't populate the typed fields.
func genericAmount(in CreateTipInput) int64 {
	switch {
	case in.PointsAmount > 0:
		return in.PointsAmount
	case in.CreditsAmount > 0:
		return in.CreditsAmount
	default:
		return in.FiatAmount
	}
}

func nullableOwnerUser(o Owner) pgtype.UUID {
	if o.Type == RecipientUser {
		return o.ID
	}
	return pgtype.UUID{}
}

func nullableOwnerOrg(o Owner) pgtype.UUID {
	if o.Type == RecipientOrganization {
		return o.ID
	}
	return pgtype.UUID{}
}

// notifyReceived fires the tip.received fan-out — fire-and-forget (Rule 8).
func (s *Service) notifyReceived(ctx context.Context, tipID pgtype.UUID) {
	if s.notifier != nil {
		_ = s.notifier.NotifyTipReceived(ctx, tipID)
	}
}

// tipFromRow maps a gen.Tip row onto the service DTO.
func tipFromRow(t gen.Tip) Tip {
	return Tip{
		ID:              t.ID,
		SenderUserID:    t.SenderUserID,
		SenderName:      senderName(t),
		RecipientType:   t.RecipientType,
		RecipientUserID: t.RecipientUserID,
		RecipientOrgID:  t.RecipientOrganizationID,
		ContentID:       t.ContentID,
		Method:          t.Method,
		PointsAmount:    t.PointsAmount,
		CreditsAmount:   t.CreditsAmount,
		FiatAmount:      t.FiatAmount,
		FiatCurrency:    t.FiatCurrency,
		FeeAmount:       t.FeeAmount,
		NetCredits:      t.NetCredits,
		PaymentID:       t.PaymentID,
		Message:         t.Message,
		IsAnonymous:     t.IsAnonymous,
		Status:          t.Status,
		CreatedAt:       t.CreatedAt,
	}
}

func senderName(t gen.Tip) string {
	if t.IsAnonymous {
		return "Someone"
	}
	if t.ExternalSenderName != nil {
		return *t.ExternalSenderName
	}
	return ""
}
