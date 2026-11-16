package stablecoin

import (
	"context"
	"fmt"
)

// IngestWebhook authenticates and queues one provider event. Deduped by
// tx signature (jc:stablecoin:webhook:dedupe:{sig}) then handed to
// stablecoin:sync_activity — fan-out, never blocking the webhook response
// (Rule 8).
func (s *Service) IngestWebhook(ctx context.Context, payload []byte, signature string) error {
	if err := s.provider.VerifyWebhook(payload, signature); err != nil {
		return fmt.Errorf("stablecoin: webhook auth: %w", err)
	}
	// TODO(§128.9): extract event tx_signature → SET NX dedupe → enqueue
	// stablecoin:sync_activity. DB unique key on tx_signature is the
	// authoritative dedupe; Valkey is the fast path.
	return nil
}

// MirrorActivity persists one provider event into stablecoin_activity —
// the render cache for the unified history feed. Called by
// stablecoin:sync_activity.
func (s *Service) MirrorActivity(ctx context.Context, ev ProviderEvent, walletID string) error {
	if _, err := s.store.GetActivityBySig(ctx, ev.TxSignature); err == nil {
		return nil // already mirrored — idempotent
	}
	_, err := s.store.InsertActivity(ctx, Activity{
		WalletID:    walletID,
		TxSignature: ev.TxSignature,
		Asset:       ev.Asset,
		AmountMinor: ev.AmountMinor,
		Status:      "pending",
		// TODO(§128.9): map ProviderEvent.Type → kind/direction,
		// credits_amount at event-time credits_per_unit ratio,
		// counterparty resolution (member wallet ↔ external).
	})
	return err
}

// Reconcile is the periodic mirror-vs-chain sweep
// (stablecoin:reconcile_wallets) — closes webhook gaps, flags divergent
// balances. Chain is truth; the mirror is a render cache.
func (s *Service) Reconcile(ctx context.Context) error {
	// TODO(§128.9): for each active wallet, provider.Activity(locator) →
	// upsert missing mirror rows → recompute mirror cache → alert on
	// divergence beyond dust threshold.
	return nil
}
