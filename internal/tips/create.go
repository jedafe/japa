// create.go — §125 POST /tips: recipient resolution, validation, and the
// credits settlement path (fiat branches into fiat.go). Points are not a
// tip method (ADR-136).
package tips

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/wallet"
)

// CreateTip validates and settles a tip. Credits settle synchronously via
// the wallet ledger pair; fiat methods create a pending tip + payment row
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
			return CreateTipResult{Tip: tipFromRow(existing), CheckoutURL: derefStr(existing.CheckoutUrl), Duplicate: true}, nil
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

	// §125 recipient eligibility is role-gated for member recipients: the
	// wallet owner must hold tips.receive or be a mentor — checked live so a
	// revoked-permission recipient's enabled settings row can't keep
	// collecting tips. Organizations opt in via their own org-admin-managed
	// settings row (spec's 'organization' grant), so the org owner's personal
	// tier is not a gate. Platform requires no check.
	if owner.Type == RecipientUser {
		ok, err := s.CanReceive(ctx, walletOwner)
		if err != nil {
			return CreateTipResult{}, err
		}
		if !ok {
			return CreateTipResult{}, ErrNotEligible
		}
	}

	if len(in.Message) > maxTipMessageLen {
		return CreateTipResult{}, ErrMessageTooLong
	}

	// §41: blocked pairs cannot transact — checked against the resolved
	// wallet owner so content-anchored tips (author) and org tips (org
	// owner's wallet) can't bypass the direct-recipient check.
	if walletOwner.Valid && walletOwner != senderID {
		if blocked, bErr := s.IsBlocked(ctx, senderID, walletOwner); bErr == nil && blocked {
			return CreateTipResult{}, ErrBlocked
		}
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
	case MethodCredits:
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
	// Content-anchored tip: recipient is the content's author. A conflicting
	// recipient_type (platform/organization) is rejected — the donor's choice
	// can't silently land on a different owner.
	if in.ContentID.Valid {
		if in.RecipientType != "" && in.RecipientType != RecipientUser {
			return Owner{}, pgtype.UUID{}, ErrInvalidRecipient
		}
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
	case RecipientSpace:
		if !in.RecipientSpaceID.Valid {
			return Owner{}, pgtype.UUID{}, ErrInvalidRecipient
		}
		walletOwnerID, err := s.store.getSpaceTipRecipient(ctx, in.RecipientSpaceID)
		if err != nil {
			return Owner{}, pgtype.UUID{}, err
		}
		return SpaceOwner(in.RecipientSpaceID), walletOwnerID, nil
	case RecipientPlatform:
		return PlatformOwner(), pgtype.UUID{}, nil
	default:
		return Owner{}, pgtype.UUID{}, ErrInvalidRecipient
	}
}

// createWalletTip runs the synchronous credits path (ADR-136 — the only
// wallet-ledger method; points are retired).
func (s *Service) createWalletTip(ctx context.Context, senderID pgtype.UUID, owner Owner, walletOwner pgtype.UUID, st Settings, in CreateTipInput, idemKey string) (CreateTipResult, error) {
	// Platform has no wallet — credits tips to it are invalid.
	if owner.Type == RecipientPlatform {
		return CreateTipResult{}, ErrMethodNotAccepted
	}
	// Orgs without a resolved owner wallet can't settle either.
	if !walletOwner.Valid {
		return CreateTipResult{}, ErrInvalidRecipient
	}

	amount := in.CreditsAmount
	if !st.AcceptCredits {
		return CreateTipResult{}, ErrMethodNotAccepted
	}
	if amount <= 0 {
		return CreateTipResult{}, ErrInvalidAmount
	}
	if amount < st.MinTipAmount {
		return CreateTipResult{}, ErrBelowMinimum
	}

	fee := amount * int64(s.feePercent(ctx)) / 100
	net := amount - fee

	tip, err := s.store.settleWalletTip(ctx, settleWalletParams{
		senderID:         senderID,
		walletOwnerID:    walletOwner,
		recipientUserID:  nullableOwnerUser(owner),
		recipientOrgID:   nullableOwnerOrg(owner),
		recipientSpaceID: nullableOwnerSpace(owner),
		ledgerType:       "credits",
		gross:            amount,
		net:              net,
		fee:              fee,
		method:           in.Method,
		recipientType:    owner.Type,
		contentID:        in.ContentID,
		message:          in.Message,
		isAnonymous:      in.IsAnonymous,
		idempotencyKey:   idemKey,
	})
	if err != nil {
		// tips_idem_unique (000949): a concurrent same-key request that won
		// the race committed first — replay its result instead of erroring.
		if errors.Is(err, ErrDuplicateKey) && idemKey != "" {
			if existing, gerr := s.store.getTipByIdempotencyKey(ctx, senderID, idemKey); gerr == nil {
				return CreateTipResult{Tip: tipFromRow(existing), Duplicate: true}, nil
			}
		}
		return CreateTipResult{}, err
	}

	// ADR-136: the sender earns tip.sent points (engagement), the recipient
	// gets credits (transaction). Reference = tip id → exactly-once award.
	s.emitActivity(ctx, senderID, wallet.ActivityKeyTipSent, "tip:"+tip.ID.String())

	s.notifyReceived(ctx, tip.ID)
	return CreateTipResult{Tip: tipFromRow(tip)}, nil
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

func nullableOwnerSpace(o Owner) pgtype.UUID {
	if o.Type == RecipientSpace {
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

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
