// stablecoin.go — §128 Asynq handlers: wallet provisioning, activity
// sync, reconciliation, signer-scope rotation, escrow timeout, treasury
// sweep, offramp tracking, and stablecoin tip settlement. Task names are
// registered in Section 11 (Rule 7).
package worker

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
)

// Section 128 task type constants — names must match the §11 catalog.
const (
	TypeStablecoinProvisionWallet  = "stablecoin:provision_wallet"
	TypeStablecoinSyncActivity     = "stablecoin:sync_activity"
	TypeStablecoinReconcileWallets = "stablecoin:reconcile_wallets"
	TypeStablecoinSignerScopeRenew = "stablecoin:signer_scope_renew"
	TypeStablecoinEscrowTimeout    = "stablecoin:escrow_timeout"
	TypeStablecoinTreasurySweep    = "stablecoin:treasury_sweep"
	TypeStablecoinProcessOfframp   = "stablecoin:process_offramp"
	TypeTipsSettleStablecoin       = "tips:settle_stablecoin"
)

// StablecoinProcessor is the worker-side seam into
// internal/stablecoin.Service — kept narrow so worker never imports the
// domain package directly.
type StablecoinProcessor interface {
	ProcessProvisionWallet(ctx context.Context, raw []byte) error
	ProcessSyncActivity(ctx context.Context, raw []byte) error
	ProcessReconcile(ctx context.Context, raw []byte) error
	ProcessSignerScopeRenew(ctx context.Context, raw []byte) error
	ProcessEscrowTimeout(ctx context.Context, raw []byte) error
	ProcessTreasurySweep(ctx context.Context, raw []byte) error
	ProcessOfframp(ctx context.Context, raw []byte) error
	ProcessTipSettle(ctx context.Context, raw []byte) error
}

var stablecoinProcessor StablecoinProcessor

// RegisterStablecoinProcessor injects the §128 service at server boot.
func RegisterStablecoinProcessor(p StablecoinProcessor) {
	stablecoinProcessor = p
}

func stablecoinHandle(name string, fn func(context.Context, []byte) error) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		if stablecoinProcessor == nil {
			return fmt.Errorf("stablecoin processor not registered")
		}
		if err := fn(ctx, t.Payload()); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	}
}

func handleStablecoinProvisionWallet(ctx context.Context, t *asynq.Task) error {
	return stablecoinHandle(TypeStablecoinProvisionWallet, stablecoinProcessor.ProcessProvisionWallet)(ctx, t)
}

func handleStablecoinSyncActivity(ctx context.Context, t *asynq.Task) error {
	return stablecoinHandle(TypeStablecoinSyncActivity, stablecoinProcessor.ProcessSyncActivity)(ctx, t)
}

func handleStablecoinReconcileWallets(ctx context.Context, t *asynq.Task) error {
	return stablecoinHandle(TypeStablecoinReconcileWallets, stablecoinProcessor.ProcessReconcile)(ctx, t)
}

func handleStablecoinSignerScopeRenew(ctx context.Context, t *asynq.Task) error {
	return stablecoinHandle(TypeStablecoinSignerScopeRenew, stablecoinProcessor.ProcessSignerScopeRenew)(ctx, t)
}

func handleStablecoinEscrowTimeout(ctx context.Context, t *asynq.Task) error {
	return stablecoinHandle(TypeStablecoinEscrowTimeout, stablecoinProcessor.ProcessEscrowTimeout)(ctx, t)
}

func handleStablecoinTreasurySweep(ctx context.Context, t *asynq.Task) error {
	return stablecoinHandle(TypeStablecoinTreasurySweep, stablecoinProcessor.ProcessTreasurySweep)(ctx, t)
}

func handleStablecoinProcessOfframp(ctx context.Context, t *asynq.Task) error {
	return stablecoinHandle(TypeStablecoinProcessOfframp, stablecoinProcessor.ProcessOfframp)(ctx, t)
}

func handleTipsSettleStablecoin(ctx context.Context, t *asynq.Task) error {
	return stablecoinHandle(TypeTipsSettleStablecoin, stablecoinProcessor.ProcessTipSettle)(ctx, t)
}
