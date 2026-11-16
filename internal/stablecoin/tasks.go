package stablecoin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// Asynq task payloads — names registered in Section 11 (Rule 7). Handlers
// enqueue only; work happens in the worker (fire-and-forget, Rule 8).

// ProvisionWalletPayload carries stablecoin:provision_wallet.
type ProvisionWalletPayload struct {
	UserID     string `json:"user_id"`
	MemberAuth string `json:"member_auth"`
}

// SyncActivityPayload carries stablecoin:sync_activity.
type SyncActivityPayload struct {
	Event []byte `json:"event"` // raw provider event
}

// SignerScopeRenewPayload carries stablecoin:signer_scope_renew.
type SignerScopeRenewPayload struct {
	UserID string `json:"user_id"` // empty = sweep all active wallets
}

// TipSettlePayload carries tips:settle_stablecoin.
type TipSettlePayload struct {
	TipID string `json:"tip_id"`
}

// ProcessProvisionWallet handles stablecoin:provision_wallet.
func (s *Service) ProcessProvisionWallet(ctx context.Context, raw []byte) error {
	var p ProvisionWalletPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("%s: unmarshal: %w", TaskProvisionWallet, err)
	}
	var uid pgtype.UUID
	if err := uid.Scan(p.UserID); err != nil {
		return fmt.Errorf("%s: user_id: %w", TaskProvisionWallet, err)
	}
	_, err := s.ProvisionWallet(ctx, uid, p.MemberAuth)
	return err
}

// ProcessSyncActivity handles stablecoin:sync_activity.
func (s *Service) ProcessSyncActivity(ctx context.Context, raw []byte) error {
	var p SyncActivityPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("%s: unmarshal: %w", TaskSyncActivity, err)
	}
	// TODO(§128.9): decode ProviderEvent → resolve wallet → MirrorActivity.
	return nil
}

// ProcessReconcile handles stablecoin:reconcile_wallets (cron).
func (s *Service) ProcessReconcile(ctx context.Context, _ []byte) error {
	return s.Reconcile(ctx)
}

// ProcessSignerScopeRenew handles stablecoin:signer_scope_renew (cron) —
// renews delegated-signer scopes before expiresAt for active members.
func (s *Service) ProcessSignerScopeRenew(ctx context.Context, raw []byte) error {
	var p SignerScopeRenewPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("%s: unmarshal: %w", TaskSignerScopeRenew, err)
	}
	// TODO(§128.2): list wallets with scope expiry < renew window →
	// provider renewal → persist new expiresAt.
	return nil
}

// ProcessEscrowTimeout handles stablecoin:escrow_timeout (cron).
func (s *Service) ProcessEscrowTimeout(ctx context.Context, _ []byte) error {
	return s.HandleEscrowTimeout(ctx)
}

// ProcessTreasurySweep handles stablecoin:treasury_sweep (cron).
func (s *Service) ProcessTreasurySweep(ctx context.Context, _ []byte) error {
	return s.ProposeSweep(ctx)
}

// ProcessOfframp handles stablecoin:process_offramp — tracks fiat-offramp
// payout status and settles the mirror row.
func (s *Service) ProcessOfframp(ctx context.Context, _ []byte) error {
	// TODO(§128.6): poll provider payout status → settle activity rows.
	return nil
}

// ProcessTipSettle handles tips:settle_stablecoin — onchain tip settlement
// via delegated signer (tip−fee → recipient, fee → treasury hot).
func (s *Service) ProcessTipSettle(ctx context.Context, raw []byte) error {
	var p TipSettlePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("%s: unmarshal: %w", TaskTipSettleStablecoin, err)
	}
	// TODO(§125/§128.5): load tip → Settle with fee leg (stablecoin.tip_fee_bps).
	return nil
}
